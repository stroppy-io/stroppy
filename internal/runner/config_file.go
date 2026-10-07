package runner

import (
	"encoding/json"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/common/logger"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

// DefaultConfigFile is the file auto-discovered in the current directory.
const DefaultConfigFile = "stroppy-config.json"

// LoadedConfig keeps the run config separate from typed parameter scopes.
type LoadedConfig struct {
	Path      string
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

	cfg := &config.RunConfig{}
	if err := UnmarshalStrict(data, cfg); err != nil {
		return nil, false, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	runParams := cfg.Run
	if runParams == nil {
		runParams = map[string]json.RawMessage{}
	}

	workloadParams := cfg.Params
	if workloadParams == nil {
		workloadParams = map[string]json.RawMessage{}
	}

	return &LoadedConfig{Path: path, RunConfig: cfg, Run: runParams, Params: workloadParams}, true, nil
}

// LogConfigFile emits config-file diagnostics after logger initialization.
func LogConfigFile(loaded *LoadedConfig) {
	if loaded == nil || loaded.RunConfig == nil {
		return
	}

	cfg := loaded.RunConfig
	lg := logger.Global().Named("config_file")
	lg.Info("Loaded config file", zap.String("path", loaded.Path))

	if cfg.GetScript() != "" {
		lg.Debug("Config file script", zap.String("script", cfg.GetScript()))
	}

	for idx, drv := range cfg.Drivers {
		lg.Debug("Config file driver",
			zap.String("name", idx),
			zap.String("type", drv.GetDriverType()),
			zap.String("url", logger.RedactDSN(drv.GetURL())),
		)
	}
}
