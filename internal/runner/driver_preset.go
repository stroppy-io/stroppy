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
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// pathFields lists Extra keys that contain file paths and must be
// resolved to absolute paths before the working directory changes.
var pathFields = map[string]bool{
	"cacertfile": true,
}

var (
	errUnknownDriver          = errors.New("unknown driver")
	errInvalidDriverOverride  = errors.New("invalid driver override")
	errDriverOverrideConflict = errors.New("driver override conflicts with existing non-object value")
)

const (
	driverTypeKey          = "drivertype"
	urlKey                 = "url"
	defaultInsertMethodKey = "defaultinsertmethod"
)

// inferType converts a CLI string value to its most specific Go type
// so that JSON serialization emits a number/bool instead of a quoted string.
// This is required because protobuf (TS side) rejects "20" for int32 fields.
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
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   host,
	}).String()
}

// driverPresets maps short driver names to their default configurations.
var driverPresets = map[string]DriverPreset{
	"pg": {
		DriverType:          "postgres",
		URL:                 postgresURL("postgres", "postgres", "localhost:5432"),
		DefaultInsertMethod: "native",
		PoolKind:            "postgres",
	},
	"mysql": {
		DriverType: "mysql",
		URL: "myuser:mypassword@tcp(localhost:3306)" +
			"/mydb?charset=utf8mb4&parseTime=True&loc=Local",
		DefaultInsertMethod: "plain_bulk",
		PoolKind:            "sql",
	},
	"pico": {
		DriverType:          "picodata",
		URL:                 postgresURL("admin", "T0psecret", "localhost:1331"),
		DefaultInsertMethod: "plain_bulk",
		PoolKind:            "postgres",
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
		DefaultInsertMethod: "plain_bulk",
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

// DriverCLIConfig represents a fully resolved driver configuration from CLI flags.
// It is serialized to JSON and passed as STROPPY_DRIVER_N env var to the k6 script.
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

// ApplyOverride sets a field by key=value. Known fields are set on the struct,
// unknown fields go into Extra for pass-through to TS.
func (d *DriverCLIConfig) ApplyOverride(key, value string) error {
	if key == "" {
		return fmt.Errorf("%w: empty key", errInvalidDriverOverride)
	}

	switch normalizeKey(key) {
	case driverTypeKey:
		d.DriverType = value
	case urlKey:
		d.URL = value
	case defaultInsertMethodKey:
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
	switch normalizeKey(key) {
	case driverTypeKey, urlKey, defaultInsertMethodKey:
		return true
	default:
		return false
	}
}

func convertOverrideValue(key, value string) any {
	if pathFields[normalizeKey(key)] {
		if abs, err := filepath.Abs(value); err == nil {
			return abs
		}
	}

	return inferType(value)
}

func normalizeKey(key string) string {
	replacer := strings.NewReplacer("_", "", "-", "")

	return strings.ToLower(replacer.Replace(key))
}

// NewDriverCLIConfigFromPreset creates a DriverCLIConfig from a preset.
func NewDriverCLIConfigFromPreset(p DriverPreset) DriverCLIConfig {
	return DriverCLIConfig{
		DriverType:          p.DriverType,
		URL:                 p.URL,
		DefaultInsertMethod: p.DefaultInsertMethod,
	}
}

// NewDriverCLIConfigFromJSON creates a DriverCLIConfig from a raw JSON string.
// Known fields are extracted into the struct, everything else goes into Extra.
func NewDriverCLIConfigFromJSON(raw string) (DriverCLIConfig, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return DriverCLIConfig{}, fmt.Errorf("invalid driver JSON: %w", err)
	}

	cfg := DriverCLIConfig{}

	for field, val := range m {
		str, _ := val.(string)

		switch normalizeKey(field) {
		case driverTypeKey:
			cfg.DriverType = str
		case urlKey:
			cfg.URL = str
		case defaultInsertMethodKey:
			cfg.DefaultInsertMethod = str
		default:
			if cfg.Extra == nil {
				cfg.Extra = make(map[string]any)
			}

			if pathFields[normalizeKey(field)] {
				if s, ok := val.(string); ok {
					if abs, err := filepath.Abs(s); err == nil {
						val = abs
					}
				}
			}

			cfg.Extra[field] = val
		}
	}

	return cfg, nil
}

// DriverCLIConfigs holds parsed driver configurations indexed by driver number.
type DriverCLIConfigs map[int]*DriverCLIConfig

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
// protojson produces camelCase field names matching the TypeScript DriverSetup
// interface consumed by declareDriverSetup() in helpers.ts.
func fileDriverRunConfigsToEnvVars(
	fileDrivers map[uint32]*stroppy.DriverRunConfig,
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

		data, err := (protojson.MarshalOptions{EmitUnpopulated: false}).Marshal(drCfg)
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
