// Package probe contains command to get metainformation about test.
package probe

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/internal/runner"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/probe"
	"github.com/stroppy-io/stroppy/workloads"
)

const (
	maxArgs = 2
	minArgs = 1

	localFlag  = "local"
	formatFlag = "output"
	fileFlag   = "file"

	configFlag  = "config"
	optionsFlag = "options"
	sqlFlag     = "sql"
	stepsFlag   = "steps"
	envsFlag    = "envs"
	driversFlag = "drivers"

	humanFormat = "human"
	jsonFormat  = "json"
)

var (
	formats             = []string{humanFormat, jsonFormat}
	formatsWithCommas   = strings.Join(formats, ", ")
	ErrUnsoportedFormat = errors.New("unsupported format")
	Cmd                 = func() *cobra.Command {
		cmd := &cobra.Command{
			Use:   "probe",
			Short: "Get test introspection, config, options, sql, steps, envs, drivers",
			Long: `Probe runs a test script in a mocked environment to extract metadata
without executing the actual benchmark. Shows configuration, k6 options,
SQL structure, steps, environment variables, and driver setup.

With no script argument, probe lists the embedded presets, which of
their scripts are runnable, and the insert methods each driver
supports (-o json for machine output).

Use section flags (--config, --options, --sql, --steps, --envs, --drivers)
to filter output. See 'stroppy help probe' for section descriptions.
`,
			Args: cobra.RangeArgs(0, maxArgs),
			RunE: func(cmd *cobra.Command, args []string) error {
				localFlagValue, _ := cmd.Flags().GetBool(localFlag)
				formatFlagValue := cmd.Flag(formatFlag).Value.String()
				fileFlagValue := cmd.Flag(fileFlag).Value.String()

				if !slices.Contains(formats, formatFlagValue) {
					return fmt.Errorf(
						"%q, available (%s): %w",
						formatFlagValue,
						formatsWithCommas,
						ErrUnsoportedFormat,
					)
				}

				// Load config file if -f is specified or stroppy-config.json exists.
				fileConfig, _, err := runner.LoadRunConfig(fileFlagValue)
				if err != nil {
					return fmt.Errorf("failed to load config file: %w", err)
				}

				// Determine script and SQL paths: CLI args override file config.
				var (
					scriptPath string
					sqlPath    string
				)

				if len(args) >= minArgs {
					scriptPath = args[minArgs-1]
				}

				if len(args) == maxArgs {
					sqlPath = args[maxArgs-1]
				}

				scriptPath = runner.EffectiveScript(scriptPath, fileConfig)
				sqlPath = runner.EffectiveSQL(sqlPath, fileConfig)

				// No script anywhere → show the catalog of embedded presets and
				// which of their scripts are runnable.
				if scriptPath == "" {
					return printCatalog(formatFlagValue)
				}

				probeEnv, err := runner.BuildProbeEnvFromRunConfig(fileConfig)
				if err != nil {
					return fmt.Errorf("failed to build probe env from config file: %w", err)
				}

				var probeprint *runner.Probeprint

				if localFlagValue {
					probeprint, err = runner.ProbeScriptWithEnv(scriptPath, probeEnv)
				} else {
					probeprint, err = probe.ScriptInTmpWithEnv(scriptPath, sqlPath, probeEnv)
				}

				if err != nil {
					return fmt.Errorf("error while probbing %q: %w", scriptPath, err)
				}

				if fileConfig != nil {
					probeprint.FileConfig = fileConfig
				}

				sections := buildSections(cmd)

				switch formatFlagValue {
				case humanFormat:
					fmt.Fprintf(os.Stdout, "\n%s\n", probeprint.Explain(sections))
				case jsonFormat:
					bytes, err := json.Marshal(probeprint)
					if err != nil {
						return fmt.Errorf("can't marshal %#v: %w", probeprint, err)
					}

					fmt.Fprintf(os.Stdout, "\n%s\n", string(bytes))
				}

				return nil
			},
		}

		cmd.Flags().
			StringP(formatFlag, string(formatFlag[0]), humanFormat,
				fmt.Sprintf("(%s)", formatsWithCommas))

		cmd.Flags().
			BoolP(localFlag, string(localFlag[0]), false,
				"prevent tmp dir creation (use local dependencies in test working directory)")

		cmd.Flags().
			StringP(fileFlag, string(fileFlag[0]), "",
				"load config from file (default: ./stroppy-config.json if present)")

		cmd.Flags().Bool(configFlag, false, "show only stroppy config section")
		cmd.Flags().Bool(optionsFlag, false, "show only k6 options section")
		cmd.Flags().Bool(sqlFlag, false, "show only sql file structure section")
		cmd.Flags().Bool(stepsFlag, false, "show only steps section")
		cmd.Flags().Bool(envsFlag, false, "show only environment variables section")
		cmd.Flags().Bool(driversFlag, false, "show only drivers section")

		return cmd
	}()
)

// buildSections maps bool flags to ExplainSection bitmask.
// If no section flags are set, all sections are included.
func buildSections(cmd *cobra.Command) runner.ExplainSection {
	flagMap := []struct {
		name    string
		section runner.ExplainSection
	}{
		{configFlag, runner.ExplainConfig},
		{optionsFlag, runner.ExplainOptions},
		{sqlFlag, runner.ExplainSQL},
		{stepsFlag, runner.ExplainSteps},
		{envsFlag, runner.ExplainEnvs},
		{driversFlag, runner.ExplainDrivers},
	}

	var sections runner.ExplainSection

	for _, f := range flagMap {
		if v, _ := cmd.Flags().GetBool(f.name); v {
			sections |= f.section
		}
	}

	if sections == 0 {
		return runner.ExplainAll
	}

	return sections
}

// printCatalog renders the embedded preset catalog and the driver
// insert-method matrix in the requested format.
func printCatalog(format string) error {
	catalog, err := workloads.Catalog()
	if err != nil {
		return fmt.Errorf("failed to build workloads catalog: %w", err)
	}

	drivers := driverCatalog()

	switch format {
	case jsonFormat:
		bytes, err := json.Marshal(map[string]any{"presets": catalog, "drivers": drivers})
		if err != nil {
			return fmt.Errorf("can't marshal catalog: %w", err)
		}

		fmt.Fprintf(os.Stdout, "%s\n", string(bytes))
	case humanFormat:
		fmt.Fprint(os.Stdout, formatCatalog(catalog, drivers))
	}

	return nil
}

// driverEntry is one row of the driver capability matrix in catalog output.
type driverEntry struct {
	Type          string   `json:"type"`
	InsertMethods []string `json:"insert_methods"`
}

// driverCatalog converts the static driver→insert-method matrix to
// lowercase names ("postgres", "plain_bulk") for catalog output.
func driverCatalog() []driverEntry {
	capabilities := driver.InsertCapabilities()

	entries := make([]driverEntry, 0, len(capabilities))

	for _, capability := range capabilities {
		methods := make([]string, 0, len(capability.InsertMethods))
		for _, method := range capability.InsertMethods {
			methods = append(methods, strings.ToLower(method.String()))
		}

		entries = append(entries, driverEntry{
			Type: strings.ToLower(
				strings.TrimPrefix(capability.Type.String(), "DRIVER_TYPE_"),
			),
			InsertMethods: methods,
		})
	}

	return entries
}

// formatCatalog builds the human-readable preset listing and driver
// insert-method matrix.
func formatCatalog(catalog []workloads.PresetInfo, drivers []driverEntry) string {
	var builder strings.Builder

	builder.WriteString("\nPRESETS (embedded workloads)\n\n")

	for _, preset := range catalog {
		builder.WriteString("  " + preset.Name + "\n")

		var runnable []string

		for _, script := range preset.Scripts {
			if script.Runnable {
				runnable = append(runnable, script.Name)
			}
		}

		if len(runnable) > 0 {
			builder.WriteString("    scripts:  " + strings.Join(runnable, ", ") + "\n")
		}

		if len(preset.SQL) > 0 {
			builder.WriteString("    sql:      " + strings.Join(preset.SQL, ", ") + "\n")
		}

		if len(preset.Docs) > 0 {
			builder.WriteString("    docs:     " + strings.Join(preset.Docs, ", ") + "\n")
		}

		if ex := runExample(preset.Name, runnable); ex != "" {
			builder.WriteString("    run:      " + ex + "\n")
		}

		builder.WriteString("\n")
	}

	builder.WriteString("DRIVERS (supported insert methods)\n\n")

	typeWidth := 0
	for _, entry := range drivers {
		typeWidth = max(typeWidth, len(entry.Type))
	}

	for _, entry := range drivers {
		fmt.Fprintf(&builder, "  %-*s  %s\n",
			typeWidth, entry.Type, strings.Join(entry.InsertMethods, ", "))
	}

	builder.WriteString("\nProbe any script for details:  stroppy probe <preset>/<script>\n")

	return builder.String()
}

// runExample returns a ready-to-copy run command for the first runnable script.
func runExample(preset string, runnable []string) string {
	if len(runnable) == 0 {
		return ""
	}

	stem := strings.TrimSuffix(runnable[0], ".ts")
	if stem == preset {
		return "stroppy run " + preset
	}

	return "stroppy run " + preset + "/" + stem
}
