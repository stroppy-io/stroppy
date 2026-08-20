package runner

// EffectiveScript returns the script to use.
// CLI positional arg takes precedence over config file.
func EffectiveScript(cliScript string, cfg *LoadedConfig) string {
	if cliScript != "" {
		return cliScript
	}

	if cfg != nil && cfg.RunConfig != nil && cfg.RunConfig.Script != nil {
		return cfg.RunConfig.GetScript()
	}

	return ""
}

// EffectiveSQL returns the SQL arg to use.
// CLI positional arg takes precedence over config file.
func EffectiveSQL(cliSQL string, cfg *LoadedConfig) string {
	if cliSQL != "" {
		return cliSQL
	}

	if cfg != nil && cfg.RunConfig != nil && cfg.RunConfig.Sql != nil {
		return cfg.RunConfig.GetSql()
	}

	return ""
}

// EffectiveSteps returns the step allowlist.
// CLI --steps fully overrides config file steps.
func EffectiveSteps(cliSteps []string, cfg *LoadedConfig) []string {
	if len(cliSteps) > 0 {
		return cliSteps
	}

	if cfg != nil && cfg.RunConfig != nil {
		return cfg.RunConfig.Steps
	}

	return nil
}

// EffectiveNoSteps returns the step blocklist.
// CLI --no-steps fully overrides config file no_steps.
func EffectiveNoSteps(cliNoSteps []string, cfg *LoadedConfig) []string {
	if len(cliNoSteps) > 0 {
		return cliNoSteps
	}

	if cfg != nil && cfg.RunConfig != nil {
		return cfg.RunConfig.NoSteps
	}

	return nil
}
