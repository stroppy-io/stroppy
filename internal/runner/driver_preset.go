package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
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

var (
	errUnknownDriver          = errors.New("unknown driver")
	errInvalidDriverOverride  = errors.New("invalid driver override")
	errDriverOverrideConflict = errors.New("driver override conflicts with existing non-object value")
	errNilDriverConfig        = errors.New("nil driver config")
)

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

	// Extra fields from config-file drivers that don't map to known fields.
	Extra map[string]any `json:"-"`

	// Overrides retains -D occurrences for strict, order-preserving decoding.
	Overrides []DriverOverride `json:"-"`
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

// DriverOverride is one -D key=value occurrence.
type DriverOverride struct {
	Key   string
	Value string
}

// ApplyOverride retains a driver override for strict decoding at runtime.
func (d *DriverCLIConfig) ApplyOverride(key, value string) error {
	if key == "" {
		return fmt.Errorf("%w: empty key", errInvalidDriverOverride)
	}

	path := strings.Split(key, ".")
	if err := validateOverridePath(path); err != nil {
		return err
	}

	switch key {
	case "driverType", "driver_type":
		d.DriverType = value
	case "url":
		d.URL = value
	case "defaultInsertMethod", "default_insert_method":
		d.DefaultInsertMethod = value
	default:
		if err := d.setExtraPath(path, driverOverrideValue(value)); err != nil {
			return err
		}
	}

	d.Overrides = append(d.Overrides, DriverOverride{Key: key, Value: value})

	return nil
}

func validateOverridePath(path []string) error {
	for _, part := range path {
		if part == "" {
			return fmt.Errorf("%w: empty dotted path segment", errInvalidDriverOverride)
		}
	}

	if len(path) > 1 && isDriverCLIField(path[0]) {
		return fmt.Errorf("%w: nested driver field %q", errInvalidDriverOverride, path[0])
	}

	return nil
}

func (d *DriverCLIConfig) setExtraPath(path []string, value any) error {
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

func driverOverrideValue(value string) any {
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	if looksNumeric(value) {
		return json.Number(value)
	}

	return value
}

func isDriverCLIField(key string) bool {
	switch key {
	case "driverType", "driver_type", "url", "defaultInsertMethod", "default_insert_method":
		return true
	default:
		return false
	}
}

// DecodeOverrides validates retained -D input with the shared config decoder.
func (d DriverCLIConfig) DecodeOverrides() (*config.DriverRunConfig, error) {
	if len(d.Overrides) == 0 {
		return nil, nil
	}

	data, err := marshalDriverOverrides(d.Overrides)
	if err != nil {
		return nil, err
	}

	cfg := &config.DriverRunConfig{}
	if err := config.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid driver override: %w", err)
	}

	resolveDriverConfigPaths(cfg)

	return cfg, nil
}

type driverOverrideNode struct {
	fields []driverOverrideField
}

type driverOverrideField struct {
	name  string
	value *string
	node  *driverOverrideNode
}

func marshalDriverOverrides(overrides []DriverOverride) ([]byte, error) {
	root := &driverOverrideNode{}
	for _, override := range overrides {
		root.add(strings.Split(override.Key, "."), override.Value)
	}

	var out bytes.Buffer
	if err := root.writeJSON(&out); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

func (node *driverOverrideNode) add(path []string, value string) {
	for index, part := range path {
		if index == len(path)-1 {
			node.fields = append(node.fields, driverOverrideField{name: part, value: &value})

			return
		}

		child := node.object(part)
		if child == nil {
			child = &driverOverrideNode{}
			node.fields = append(node.fields, driverOverrideField{name: part, node: child})
		}

		node = child
	}
}

func (node *driverOverrideNode) object(name string) *driverOverrideNode {
	for index := range node.fields {
		field := &node.fields[index]
		if field.name == name && field.node != nil {
			return field.node
		}
	}

	return nil
}

func (node *driverOverrideNode) writeJSON(out *bytes.Buffer) error {
	out.WriteByte('{')
	for index, field := range node.fields {
		if index > 0 {
			out.WriteByte(',')
		}

		name, err := json.Marshal(field.name)
		if err != nil {
			return err
		}
		out.Write(name)
		out.WriteByte(':')

		if field.node != nil {
			if err := field.node.writeJSON(out); err != nil {
				return err
			}

			continue
		}

		writeDriverOverrideValue(out, *field.value)
	}
	out.WriteByte('}')

	return nil
}

func writeDriverOverrideValue(out *bytes.Buffer, value string) {
	if value == "true" || value == "false" || looksNumeric(value) {
		out.WriteString(value)

		return
	}

	encoded, _ := json.Marshal(value)
	out.Write(encoded)
}

func looksNumeric(value string) bool {
	index := 0
	if len(value) > 0 && (value[0] == '-' || value[0] == '+') {
		index++
	}

	if index == len(value) || (value[index] < '0' || value[index] > '9') &&
		(value[index] != '.' || index+1 == len(value) || value[index+1] < '0' || value[index+1] > '9') {
		return false
	}

	for ; index < len(value); index++ {
		char := value[index]
		if char >= '0' && char <= '9' || char == '.' || char == 'e' || char == 'E' || char == '+' || char == '-' {
			continue
		}

		return false
	}

	return true
}

func resolveDriverConfigPaths(fileConfig *config.DriverRunConfig) {
	if fileConfig.CaCertFile != nil {
		if absolute, err := filepath.Abs(*fileConfig.CaCertFile); err == nil {
			fileConfig.CaCertFile = &absolute
		}
	}
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

	resolveDriverConfigPaths(&extraConfig)

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
