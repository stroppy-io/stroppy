package runner

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// DefaultConfigFile is the file auto-discovered in the current directory.
const DefaultConfigFile = "stroppy-config.json"

// LoadRunConfig loads a RunConfig from a JSON file.
//
//   - If path is non-empty: load from that path; return error if not found.
//   - If path is empty: try DefaultConfigFile in cwd; return (nil, false, nil) if absent.
//
// Returns (config, loaded, error).
func LoadRunConfig(path string) (*stroppy.RunConfig, bool, error) {
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

	cfg := &stroppy.RunConfig{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(data, cfg); err != nil {
		return nil, false, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	// Uppercase all env keys for consistency with -e flag behavior.
	if len(cfg.GetEnv()) > 0 {
		normalized := make(map[string]string, len(cfg.GetEnv()))
		for k, v := range cfg.GetEnv() {
			normalized[strings.ToUpper(k)] = v
		}

		cfg.Env = normalized
	}

	lg := logger.Global().Named("config_file")
	lg.Info("Loaded config file", zap.String("path", path))

	if cfg.GetScript() != "" {
		lg.Debug("Config file script", zap.String("script", cfg.GetScript()))
	}

	if len(cfg.GetEnv()) > 0 {
		keys := make([]string, 0, len(cfg.GetEnv()))
		for k := range cfg.GetEnv() {
			keys = append(keys, k)
		}

		sort.Strings(keys)
		lg.Debug("Config file env overrides", zap.Strings("keys", keys))
	}

	for idx, drv := range cfg.GetDrivers() {
		lg.Debug("Config file driver",
			zap.Uint32("index", idx),
			zap.String("type", drv.GetDriverType()),
			zap.String("url", drv.GetUrl()),
		)
	}

	return cfg, true, nil
}
