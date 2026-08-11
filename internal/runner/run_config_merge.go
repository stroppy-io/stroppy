package runner

import (
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// EffectiveScript returns the script to use.
// CLI positional arg takes precedence over config file.
func EffectiveScript(cliScript string, cfg *stroppy.RunConfig) string {
	if cliScript != "" {
		return cliScript
	}

	if cfg != nil && cfg.Script != nil {
		return cfg.GetScript()
	}

	return ""
}

// EffectiveSQL returns the SQL arg to use.
// CLI positional arg takes precedence over config file.
func EffectiveSQL(cliSQL string, cfg *stroppy.RunConfig) string {
	if cliSQL != "" {
		return cliSQL
	}

	if cfg != nil && cfg.Sql != nil {
		return cfg.GetSql()
	}

	return ""
}

// EffectiveSteps returns the step allowlist.
// CLI --steps fully overrides config file steps.
func EffectiveSteps(cliSteps []string, cfg *stroppy.RunConfig) []string {
	if len(cliSteps) > 0 {
		return cliSteps
	}

	if cfg != nil {
		return cfg.GetSteps()
	}

	return nil
}

// EffectiveNoSteps returns the step blocklist.
// CLI --no-steps fully overrides config file no_steps.
func EffectiveNoSteps(cliNoSteps []string, cfg *stroppy.RunConfig) []string {
	if len(cliNoSteps) > 0 {
		return cliNoSteps
	}

	if cfg != nil {
		return cfg.GetNoSteps()
	}

	return nil
}
