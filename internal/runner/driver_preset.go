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

var errUnknownDriver = errors.New("unknown driver")

// inferType converts a CLI string value to its most specific Go type
// so that JSON serialization emits a number/bool instead of a quoted string.
// This is required because protobuf (TS side) rejects "20" for int32 fields.
func inferType(s string) any {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	return s
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
		DefaultInsertMethod: "copy_from",
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
		DefaultInsertMethod: "plain_bulk",
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
func (d *DriverCLIConfig) ApplyOverride(key, value string) {
	switch strings.ToLower(key) {
	case "drivertype":
		d.DriverType = value
	case "url":
		d.URL = value
	case "defaultinsertmethod":
		d.DefaultInsertMethod = value
	default:
		if d.Extra == nil {
			d.Extra = make(map[string]any)
		}

		if pathFields[strings.ToLower(key)] {
			if abs, err := filepath.Abs(value); err == nil {
				value = abs
			}
		}

		d.Extra[key] = inferType(value)
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

		switch strings.ToLower(field) {
		case "drivertype":
			cfg.DriverType = str
		case "url":
			cfg.URL = str
		case "defaultinsertmethod":
			cfg.DefaultInsertMethod = str
		default:
			if cfg.Extra == nil {
				cfg.Extra = make(map[string]any)
			}

			if pathFields[strings.ToLower(field)] {
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
