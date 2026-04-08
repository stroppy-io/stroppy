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

// EffectiveK6Args merges config file k6_args with CLI after-dash args.
// Config file args come first so CLI args can override (last-wins for most k6 flags).
func EffectiveK6Args(cliAfterDash []string, cfg *stroppy.RunConfig) []string {
	if cfg == nil || len(cfg.GetK6Args()) == 0 {
		return cliAfterDash
	}

	if len(cliAfterDash) == 0 {
		return cfg.GetK6Args()
	}

	merged := make([]string, 0, len(cfg.GetK6Args())+len(cliAfterDash))
	merged = append(merged, cfg.GetK6Args()...)
	merged = append(merged, cliAfterDash...)

	return merged
}
