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
	"github.com/stroppy-io/stroppy/pkg/probe"
)

const (
	maxArgs = 2
	minArgs = 1

	localFlag  = "local"
	formatFlag = "output"

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
			Short: "Get test introspection, config, options, sql, steps, envs",
			Long: `Command allows you to get information about a test script without running it.
Probing performs a first-line check of the test for workability, exposing common
issues. It also provides introspection into the test configuration and
structure.

Description of -o human readable format sections provided below.

'# Stroppy Config' section shows the configuration passed to the stroppy module
via the 'DriverX.fromConfig(...)' function call. The configuration is presented
in protojson format. The config schema is defined in
'proto/stroppy/config.proto'.

'# K6 Options' follows the structure from 'k6/lib/options.go' and
'@types/k6/options/index.d.ts'. It shows the options object defined in the
script as
"import { Options } from 'k6/options';
export const options: Options = {...};"

'# SQL File Structure' section represents the SQL file required by the probed
test. Without these defined sections and queries, the test will fail.
Sections start with '--+ <SectionName>'.
Queries within sections are named '--= <QueryName>'.
You can copy-paste this text directly into an SQL file and write your queries to
match the test.

'# Steps' is logical separation of the test into steps. They are defined in the
test as 'Step("step name", ()=> {/*step code*/})' or using 'StepBegin("step
name"); StepEnd("step name");' pair.

'# Environment Variables' shows which env vars are used specifically within this
test. Only envs used as '__ENV.<env_name>' are shown here (no k6 options or
stroppy envs). Current environment values are also shown if found.

'# Drivers' shows the driver configuration(s) passed to DriverX.create().setup({...}).
It includes the driver type, connection URL, and driver-specific settings
(e.g. PostgreSQL pool configuration).
`,
			// TODO: auto detect tests with magic test.ts name.
			// Or do "probe" of this dir, go trough all ts files, show all sql, or like this.
			Args: cobra.RangeArgs(minArgs, maxArgs),
			RunE: func(cmd *cobra.Command, args []string) error {
				scriptPath := args[minArgs-1]
				sqlPath := ""
				localFlagValue, _ := cmd.Flags().GetBool(localFlag)
				formatFlagValue := cmd.Flag(formatFlag).Value.String()

				if len(args) == maxArgs {
					sqlPath = args[maxArgs-1]
				}

				if !slices.Contains(formats, formatFlagValue) {
					return fmt.Errorf(
						"%q, available (%s): %w",
						formatFlagValue,
						formatsWithCommas,
						ErrUnsoportedFormat,
					)
				}

				var (
					probeprint *runner.Probeprint
					err        error
				)

				if localFlagValue {
					probeprint, err = runner.ProbeScript(scriptPath)
				} else {
					probeprint, err = probe.ScriptInTmp(scriptPath, sqlPath)
				}

				if err != nil {
					return fmt.Errorf("error while probbing %q: %w", scriptPath, err)
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
