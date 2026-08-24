package runner

import (
	"errors"
	"fmt"
	"strings"
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
