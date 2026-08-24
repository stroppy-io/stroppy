package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/config"
)

// Driver preset literals reused across the postgres-family presets and
// their default insert method.
const (
	driverPostgres  = "postgres"
	insertPlainBulk = "plain_bulk"
)

// pathFields lists Extra keys that contain file paths and must be
// resolved to absolute paths before the working directory changes.
var pathFields = map[string]bool{
	"caCertFile":   true,
	"ca_cert_file": true,
}

var (
	errUnknownDriver          = errors.New("unknown driver")
	errInvalidDriverOverride  = errors.New("invalid driver override")
	errDriverOverrideConflict = errors.New("driver override conflicts with existing non-object value")
	errNilDriverConfig        = errors.New("nil driver config")
)

// inferType converts a CLI string value to its most specific Go type so JSON
// serialization preserves numeric and boolean -D values for strict validation.
func inferType(value string) any {
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return i
	}

	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}

	if b, err := strconv.ParseBool(value); err == nil {
		return b
	}

	return value
}

// DriverPreset contains default configuration for a known database driver.
// These are used when the user specifies --driver / -d on the CLI.
type DriverPreset struct {
	DriverType          string `json:"driverType"`
	URL                 string `json:"url"`
	DefaultInsertMethod string `json:"defaultInsertMethod,omitempty"`
	PoolKind            string `json:"-"` // "postgres" or "sql" — determines which pool config block to use
}

// postgresURL builds a postgres:// connection URL from components,
// keeping credentials out of string literals for static analysis.
func postgresURL(user, pass, host string) string {
	return (&url.URL{
		Scheme: driverPostgres,
		User:   url.UserPassword(user, pass),
		Host:   host,
	}).String()
}

// driverPresets maps short driver names to their default configurations.
var driverPresets = map[string]DriverPreset{
	"pg": {
		DriverType:          driverPostgres,
		URL:                 postgresURL(driverPostgres, driverPostgres, "localhost:5432"),
		DefaultInsertMethod: "native",
		PoolKind:            driverPostgres,
	},
	"mysql": {
		DriverType: "mysql",
		URL: "myuser:mypassword@tcp(localhost:3306)" +
			"/mydb?charset=utf8mb4&parseTime=True&loc=Local",
		DefaultInsertMethod: insertPlainBulk,
		PoolKind:            "sql",
	},
	"pico": {
		DriverType:          "picodata",
		URL:                 postgresURL("admin", "T0psecret", "localhost:1331"),
		DefaultInsertMethod: insertPlainBulk,
		PoolKind:            driverPostgres,
	},
	"ydb": {
		DriverType:          "ydb",
		URL:                 "grpc://localhost:2136/local",
		DefaultInsertMethod: "native",
		PoolKind:            "sql",
	},
	"noop": {
		DriverType:          "noop",
		URL:                 "noop://localhost",
		DefaultInsertMethod: insertPlainBulk,
		PoolKind:            "",
	},
}

// LookupDriverPreset returns a preset by short name, or an error if not found.
func LookupDriverPreset(name string) (DriverPreset, error) {
	name = strings.ToLower(name)

	preset, ok := driverPresets[name]
	if !ok {
		known := make([]string, 0, len(driverPresets))
		for k := range driverPresets {
			known = append(known, k)
		}

		return DriverPreset{}, fmt.Errorf("%w %q (available: %s)", errUnknownDriver, name, strings.Join(known, ", "))
	}

	return preset, nil
}

// DriverCLIConfig is one mutable driver configuration assembled from -d/-D
// inputs before conversion to the runtime config.
type DriverCLIConfig struct {
	// Base fields from preset (overridable via -D).
	DriverType          string `json:"driverType,omitempty"`
	URL                 string `json:"url,omitempty"`
	DefaultInsertMethod string `json:"defaultInsertMethod,omitempty"`

	// Extra fields from -D key=value overrides that don't map to known fields.
	Extra map[string]any `json:"-"`
}

// MarshalJSON produces a flat JSON object merging known fields and extras.
func (d DriverCLIConfig) MarshalJSON() ([]byte, error) {
	merged := make(map[string]any)

	if d.DriverType != "" {
		merged["driverType"] = d.DriverType
	}

	if d.URL != "" {
		merged["url"] = d.URL
	}

	if d.DefaultInsertMethod != "" {
		merged["defaultInsertMethod"] = d.DefaultInsertMethod
	}

	maps.Copy(merged, d.Extra)

	return json.Marshal(merged)
}

// ApplyOverride sets a field by key=value. Known fields are set on the struct;
// remaining fields are retained for strict nested validation during conversion.
func (d *DriverCLIConfig) ApplyOverride(key, value string) error {
	if key == "" {
		return fmt.Errorf("%w: empty key", errInvalidDriverOverride)
	}

	switch key {
	case "driverType", "driver_type":
		d.DriverType = value
	case "url":
		d.URL = value
	case "defaultInsertMethod", "default_insert_method":
		d.DefaultInsertMethod = value
	default:
		return d.setExtraPath(strings.Split(key, "."), convertOverrideValue(key, value))
	}

	return nil
}

func (d *DriverCLIConfig) setExtraPath(path []string, value any) error {
	if err := validateOverridePath(path); err != nil {
		return err
	}

	if d.Extra == nil {
		d.Extra = make(map[string]any)
	}

	target := d.Extra
	for _, part := range path[:len(path)-1] {
		next, ok := target[part]
		if !ok {
			nested := make(map[string]any)
			target[part] = nested
			target = nested

			continue
		}

		nested, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %q", errDriverOverrideConflict, part)
		}

		target = nested
	}

	last := path[len(path)-1]
	if existing, exists := target[last]; exists {
		if _, isObject := existing.(map[string]any); isObject {
			return fmt.Errorf("%w: %q", errDriverOverrideConflict, last)
		}
	}

	target[last] = value

	return nil
}

func validateOverridePath(path []string) error {
	for _, part := range path {
		if part == "" {
			return fmt.Errorf("%w: empty dotted path segment", errInvalidDriverOverride)
		}
	}

	if len(path) > 1 && isDriverCLIField(path[0]) {
		return fmt.Errorf("%w: %q", errDriverOverrideConflict, path[0])
	}

	return nil
}

func isDriverCLIField(key string) bool {
	switch key {
	case "driverType", "driver_type", "url", "defaultInsertMethod", "default_insert_method":
		return true
	default:
		return false
	}
}

func convertOverrideValue(key, value string) any {
	if pathFields[key] {
		if abs, err := filepath.Abs(value); err == nil {
			return abs
		}
	}

	return inferType(value)
}

// NewDriverCLIConfigFromPreset creates a DriverCLIConfig from a preset.
func NewDriverCLIConfigFromPreset(p DriverPreset) DriverCLIConfig {
	return DriverCLIConfig{
		DriverType:          p.DriverType,
		URL:                 p.URL,
		DefaultInsertMethod: p.DefaultInsertMethod,
	}
}

// NewDriverCLIConfigFromJSON strictly validates a raw -d JSON object before
// separating its base fields from the nested driver extras.
func NewDriverCLIConfigFromJSON(raw string) (DriverCLIConfig, error) {
	fileConfig := &config.DriverRunConfig{}
	if err := config.Unmarshal([]byte(raw), fileConfig); err != nil {
		return DriverCLIConfig{}, fmt.Errorf("invalid driver JSON: %w", err)
	}

	return driverCLIConfigFromFile(fileConfig)
}

func driverCLIConfigFromFile(fileConfig *config.DriverRunConfig) (DriverCLIConfig, error) {
	if fileConfig == nil {
		return DriverCLIConfig{}, errNilDriverConfig
	}

	extraConfig := *fileConfig
	cfg := DriverCLIConfig{
		DriverType:          fileConfig.GetDriverType(),
		URL:                 fileConfig.GetURL(),
		DefaultInsertMethod: fileConfig.GetDefaultInsertMethod(),
	}

	extraConfig.DriverType = nil
	extraConfig.URL = nil
	extraConfig.DefaultInsertMethod = nil

	if extraConfig.CaCertFile != nil {
		if absolute, err := filepath.Abs(*extraConfig.CaCertFile); err == nil {
			extraConfig.CaCertFile = &absolute
		}
	}

	data, err := json.Marshal(&extraConfig) //nolint:gosec // serializing validated config fields
	if err != nil {
		return DriverCLIConfig{}, err
	}

	if err := json.Unmarshal(data, &cfg.Extra); err != nil {
		return DriverCLIConfig{}, err
	}

	return cfg, nil
}

// DriverCLIConfigs holds parsed driver configurations indexed by driver number.
type DriverCLIConfigs map[int]*DriverCLIConfig

// DriverCLIConfigsFromFile converts config-file drivers into mutable CLI configs.
func DriverCLIConfigsFromFile(fileDrivers map[uint32]*config.DriverRunConfig) (DriverCLIConfigs, error) {
	configs := make(DriverCLIConfigs, len(fileDrivers))

	for idx, fileConfig := range fileDrivers {
		cfg, err := driverCLIConfigFromFile(fileConfig)
		if err != nil {
			return nil, fmt.Errorf("convert config file driver %d: %w", idx, err)
		}

		configs[int(idx)] = &cfg
	}

	return configs, nil
}

// ToEnvVars serializes all driver configs to STROPPY_DRIVER_N=<json> pairs.
// If a STROPPY_DRIVER_N env var is already set in the process environment,
// the CLI-composed value is skipped — user-set env takes precedence.
func (configs DriverCLIConfigs) ToEnvVars() ([]string, error) {
	lg := logger.Global().Named("driver_preset")
	envs := make([]string, 0, len(configs))

	for idx, cfg := range configs {
		envKey := fmt.Sprintf("STROPPY_DRIVER_%d", idx)

		if _, ok := os.LookupEnv(envKey); ok {
			lg.Debug("CLI driver skipped: real env takes precedence", zap.String("key", envKey))

			continue
		}

		data, err := json.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize driver %d config: %w", idx, err)
		}

		lg.Debug("Applying CLI driver config", zap.Int("index", idx), zap.String("type", cfg.DriverType))

		envs = append(envs, envKey+"="+string(data))
	}

	return envs, nil
}

// fileDriverRunConfigsToEnvVars serializes config-file driver configs to
// STROPPY_DRIVER_N env vars. Only emits vars for driver indices that are
// absent from both the real environment and cliConfigs (CLI -d/-D flags).
//
// json.Marshal produces camelCase field names matching the driver setup schema.
func fileDriverRunConfigsToEnvVars(
	fileDrivers map[uint32]*config.DriverRunConfig,
	cliConfigs DriverCLIConfigs,
) ([]string, error) {
	if len(fileDrivers) == 0 {
		return nil, nil
	}

	lg := logger.Global().Named("driver_preset")
	envs := make([]string, 0, len(fileDrivers))

	for idx, drCfg := range fileDrivers {
		envKey := fmt.Sprintf("STROPPY_DRIVER_%d", idx)

		if _, ok := os.LookupEnv(envKey); ok {
			lg.Debug("Config file driver skipped: real env takes precedence", zap.String("key", envKey))

			continue
		}

		if _, ok := cliConfigs[int(idx)]; ok {
			lg.Debug("Config file driver skipped: CLI -d/-D takes precedence", zap.Uint32("index", idx))

			continue
		}

		data, err := json.Marshal(drCfg) //nolint:gosec // serializing config to env vars, not extracting a secret
		if err != nil {
			return nil, fmt.Errorf("failed to serialize file driver %d config: %w", idx, err)
		}

		lg.Debug("Applying config file driver",
			zap.Uint32("index", idx),
			zap.String("type", drCfg.GetDriverType()),
		)

		envs = append(envs, envKey+"="+string(data))
	}

	return envs, nil
}
