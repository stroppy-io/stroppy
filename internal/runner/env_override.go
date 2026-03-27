package runner

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
)

var errInvalidEnvArg = errors.New("expected KEY=VALUE format")

// ParseEnvArg splits a "KEY=VALUE" string into key and value.
// Returns an error if the string does not contain '='.
func ParseEnvArg(arg string) (key, value string, err error) {
	key, value, ok := strings.Cut(arg, "=")
	if !ok || key == "" {
		return "", "", fmt.Errorf("invalid env arg %q: %w", arg, errInvalidEnvArg)
	}

	return key, value, nil
}

// ResolveEnvOverrides processes a slice of "KEY=VALUE" strings from -e flags,
// uppercases all keys, and returns a deduplicated map. Later values win.
func ResolveEnvOverrides(cliArgs []string) (map[string]string, error) {
	overrides := make(map[string]string, len(cliArgs))

	for _, raw := range cliArgs {
		key, value, err := ParseEnvArg(raw)
		if err != nil {
			return nil, err
		}

		overrides[strings.ToUpper(key)] = value
	}

	return overrides, nil
}

// BuildEnvLookup merges env overrides with os.Environ(), respecting precedence:
// real env (os.Environ) wins over -e overrides. Unknown keys produce a warning
// but are still included.
func BuildEnvLookup(envOverrides map[string]string) []string {
	lg := logger.Global().Named("env_override")

	// Collect already-set real env keys for precedence check.
	realEnv := make(map[string]struct{})

	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok {
			realEnv[k] = struct{}{}
		}
	}

	var result []string

	for key, value := range envOverrides {
		if _, present := realEnv[key]; present {
			lg.Warn("Ignoring -e override: real environment already sets this variable",
				zap.String("key", key),
			)

			continue
		}

		result = append(result, key+"="+value)
	}

	return result
}
