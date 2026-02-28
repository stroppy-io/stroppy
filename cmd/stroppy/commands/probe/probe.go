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

	humanFormat = "human"
	jsonFormat  = "json"
)

var (
	formats             = []string{humanFormat, jsonFormat}
	formatsWithCommas   = strings.Join(formats, ", ")
	ErrUnsoportedFormat = errors.New("unsupported format")
	Cmd                 = func() *cobra.Command {
		cmd := &cobra.Command{
			Use: "probe",
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

				switch formatFlagValue {
				case humanFormat:
					fmt.Fprintf(os.Stdout, "\n%s\n", probeprint.Explain())
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

		return cmd
	}()
)
