package bench

import (
	"os"
	"strconv"
)

// Env reads a string param. Real process env takes precedence, then the script
// env passed to Run (-e overrides + config); def when unset/empty.
func Env(name, def string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}

	if root != nil {
		if v, ok := root.env[name]; ok && v != "" {
			return v
		}
	}

	return def
}

// EnvInt reads an integer param.
func EnvInt(name string, def int) int {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		if root != nil {
			v = root.env[name]
		}
	}

	if v == "" {
		return def
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}

	return n
}

// EnvFloat reads a float param.
func EnvFloat(name string, def float64) float64 {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		if root != nil {
			v = root.env[name]
		}
	}

	if v == "" {
		return def
	}

	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}

	return f
}
