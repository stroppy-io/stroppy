package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/config"
)

// DefaultConfigFile is the file auto-discovered in the current directory.
const DefaultConfigFile = "stroppy-config.json"

var (
	errConfigObjectExpected = errors.New("JSON object expected")
	errDuplicateConfigField = errors.New("duplicate JSON field")
	errConfigEnvCollision   = errors.New("config env keys collide case-insensitively")
	errTrailingConfigData   = errors.New("trailing JSON data")
)

// LoadedConfig keeps the run config separate from typed parameter scopes.
type LoadedConfig struct {
	RunConfig *config.RunConfig
	Run       map[string]json.RawMessage
	Params    map[string]json.RawMessage
}

// LoadRunConfig loads a RunConfig from a JSON file.
//
//   - If path is non-empty: load from that path; return error if not found.
//   - If path is empty: try DefaultConfigFile in cwd; return (nil, false, nil) if absent.
//
// Returns (config, loaded, error).
func LoadRunConfig(path string) (*LoadedConfig, bool, error) {
	if path == "" {
		if _, err := os.Stat(DefaultConfigFile); os.IsNotExist(err) {
			return nil, false, nil
		}

		path = DefaultConfigFile
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("reading config file %q: %w", path, err)
	}

	fields, err := decodeRawObject(data)
	if err != nil {
		return nil, false, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	runParams, err := takeParamScope(fields, "run")
	if err != nil {
		return nil, false, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	workloadParams, err := takeParamScope(fields, "params")
	if err != nil {
		return nil, false, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	runData, err := json.Marshal(fields)
	if err != nil {
		return nil, false, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	cfg := &config.RunConfig{}
	if err := UnmarshalStrict(runData, cfg); err != nil {
		return nil, false, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	if err := normalizeRunConfigEnv(cfg); err != nil {
		return nil, false, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	lg := logger.Global().Named("config_file")
	lg.Info("Loaded config file", zap.String("path", path))

	if cfg.GetScript() != "" {
		lg.Debug("Config file script", zap.String("script", cfg.GetScript()))
	}

	if len(cfg.Env) > 0 {
		keys := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			keys = append(keys, k)
		}

		sort.Strings(keys)
		lg.Debug("Config file env overrides", zap.Strings("keys", keys))
	}

	for idx, drv := range cfg.Drivers {
		lg.Debug("Config file driver",
			zap.Uint32("index", idx),
			zap.String("type", drv.GetDriverType()),
			zap.String("url", drv.GetURL()),
		)
	}

	return &LoadedConfig{RunConfig: cfg, Run: runParams, Params: workloadParams}, true, nil
}

func normalizeRunConfigEnv(runConfig *config.RunConfig) error {
	if len(runConfig.Env) == 0 {
		return nil
	}

	keys := make([]string, 0, len(runConfig.Env))
	for key := range runConfig.Env {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	normalized := make(map[string]string, len(runConfig.Env))
	original := make(map[string]string, len(runConfig.Env))

	for _, key := range keys {
		upper := strings.ToUpper(key)
		if previous, exists := original[upper]; exists && previous != key {
			return fmt.Errorf("%w: %q and %q", errConfigEnvCollision, previous, key)
		}

		normalized[upper] = runConfig.Env[key]
		original[upper] = key
	}

	runConfig.Env = normalized

	return nil
}

func takeParamScope(fields map[string]json.RawMessage, name string) (map[string]json.RawMessage, error) {
	raw, ok := fields[name]
	if !ok {
		return map[string]json.RawMessage{}, nil
	}

	delete(fields, name)

	scope, err := decodeRawObject(raw)
	if err != nil {
		return nil, fmt.Errorf("config field %q: %w", name, err)
	}

	return scope, nil
}

func decodeRawObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}

	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, errConfigObjectExpected
	}

	fields := make(map[string]json.RawMessage)

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}

		key, ok := keyToken.(string)
		if !ok {
			return nil, errConfigObjectExpected
		}

		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("%w %q", errDuplicateConfigField, key)
		}

		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}

		fields[key] = raw
	}

	if _, err := decoder.Token(); err != nil {
		return nil, err
	}

	if err := ensureJSONEnd(decoder); err != nil {
		return nil, err
	}

	return fields, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}

	return errTrailingConfigData
}

// BuildProbeEnvFromRunConfig returns the config-derived environment that probe
// should expose through the mocked __ENV object before the script is executed.
func BuildProbeEnvFromRunConfig(cfg *config.RunConfig) (map[string]string, error) {
	if cfg == nil {
		return map[string]string{}, nil
	}

	env := make(map[string]string)

	for _, entry := range BuildFileEnvLookup(cfg.Env) {
		addEnvEntry(env, entry)
	}

	driverEnvs, err := fileDriverRunConfigsToEnvVars(cfg.Drivers, nil)
	if err != nil {
		return nil, err
	}

	for _, entry := range driverEnvs {
		addEnvEntry(env, entry)
	}

	for key := range cfg.Env {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}

	for idx := range cfg.Drivers {
		key := fmt.Sprintf("STROPPY_DRIVER_%d", idx)
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}

	if len(env) == 0 {
		return map[string]string{}, nil
	}

	return env, nil
}

func addEnvEntry(env map[string]string, entry string) {
	key, value, ok := strings.Cut(entry, "=")
	if !ok || key == "" {
		return
	}

	env[key] = value
}
