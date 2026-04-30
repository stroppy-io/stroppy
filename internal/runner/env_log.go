package runner

import (
	"sort"
	"strings"

	"go.uber.org/zap"
)

func (r *ScriptRunner) logAppliedScriptEnv(source string, envs []string) {
	entries := formatScriptEnvEntries(envs)

	r.logger.Info("Applied script env",
		zap.String("source", source),
		zap.Strings("env", entries),
	)

	unknown := unknownScriptEnvKeys(envs, r.config)
	if len(unknown) == 0 {
		return
	}

	r.logger.Warn("Script env is not declared by workload",
		zap.String("source", source),
		zap.Strings("keys", unknown),
		zap.String("hint", "check spelling or inspect the workload with `stroppy probe <script> --envs`"),
	)
}

func formatScriptEnvEntries(envs []string) []string {
	entries := make([]string, 0, len(envs))

	for _, env := range envs {
		key, value, ok := strings.Cut(env, "=")
		if !ok || key == "" {
			continue
		}

		entries = append(entries, key+"="+maskScriptEnvValue(key, value))
	}

	sort.Strings(entries)

	return entries
}

func maskScriptEnvValue(key, value string) string {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"PASSWORD", "PASS", "SECRET", "TOKEN", "CREDENTIAL", "AUTH", "URL", "DSN", "_KEY"} {
		if strings.Contains(upper, marker) {
			return "<redacted>"
		}
	}

	return value
}

func unknownScriptEnvKeys(envs []string, probe *Probeprint) []string {
	known := knownScriptEnvNames(probe)
	if len(known) == 0 {
		return nil
	}

	var unknown []string

	for _, env := range envs {
		key, _, ok := strings.Cut(env, "=")
		if !ok || key == "" {
			continue
		}

		if _, ok := known[key]; !ok {
			unknown = append(unknown, key)
		}
	}

	sort.Strings(unknown)

	return unknown
}

func knownScriptEnvNames(probe *Probeprint) map[string]struct{} {
	if probe == nil {
		return nil
	}

	known := make(map[string]struct{})

	for _, decl := range probe.EnvDeclarations {
		for _, name := range decl.Names {
			known[name] = struct{}{}
		}
	}

	for _, name := range probe.Envs {
		known[name] = struct{}{}
	}

	if len(known) == 0 {
		return nil
	}

	return known
}

func envKeySet(envs []string) map[string]struct{} {
	keys := make(map[string]struct{}, len(envs))

	for _, env := range envs {
		key, _, ok := strings.Cut(env, "=")
		if ok && key != "" {
			keys[key] = struct{}{}
		}
	}

	return keys
}

func rememberEnvKeys(keys map[string]struct{}, envs []string) {
	for _, env := range envs {
		key, _, ok := strings.Cut(env, "=")
		if ok && key != "" {
			keys[key] = struct{}{}
		}
	}
}

func keepNewEnvEntries(envs []string, known map[string]struct{}) ([]string, []string) {
	kept := make([]string, 0, len(envs))

	var skipped []string

	for _, env := range envs {
		key, _, ok := strings.Cut(env, "=")
		if !ok || key == "" {
			continue
		}

		if _, ok := known[key]; ok {
			skipped = append(skipped, key)

			continue
		}

		kept = append(kept, env)
	}

	sort.Strings(skipped)

	return kept, skipped
}
