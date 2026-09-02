package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/baseline"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/help"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/probe"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/internal/version"
	"github.com/stroppy-io/stroppy/pkg/common/shutdown"
	_ "github.com/stroppy-io/stroppy/workloads/all"
)

// appName is the binary / command name.
const appName = "stroppy"

var rootCmd = &cobra.Command{
	Use:   appName,
	Short: "Generate and run Go-native database stress tests",
}

// versionJSON controls whether `stroppy version` outputs machine-readable JSON.
// When more component versions are added (k6, drivers, etc.), --json gives
// programmatic consumers a stable format to parse instead of scraping text lines.
var versionJSON bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print versions of stroppy components",
	Run: func(_ *cobra.Command, _ []string) {
		versions := map[string]string{
			appName: version.Version,
		}

		// Pull dependency versions from the compiled binary's module info.
		// These stay in sync with go.mod automatically — no hardcoding.
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, dep := range info.Deps {
				if dep.Path == "github.com/jackc/pgx/v5" {
					versions["pgx"] = dep.Version
				}
			}
		}

		if versionJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")

			if err := enc.Encode(versions); err != nil {
				log.Fatal(err)
			}
		} else {
			// Fixed order for readable output.
			for _, kv := range []struct{ k, v string }{
				{appName, versions[appName]},
				{"pgx", versions["pgx"]},
			} {
				if kv.v != "" {
					fmt.Fprintf(os.Stdout, "%-8s %s\n", kv.k, kv.v)
				}
			}
		}
	},
}

// Execute runs the root command under a signal-derived context and maps a
// graceful cancellation to a documented exit status: the first SIGINT/SIGTERM
// cancels the command context, and a second signal forces immediate exit.
func Execute() {
	if code := execute(); code != 0 {
		os.Exit(code)
	}
}

// execute wires the cancellation context, runs Cobra, and returns the process
// exit code without terminating. Kept separate from Execute so the exit-status
// mapping is a pure function of the returned error. exitStatus is read only
// after the command returns, so it reflects whichever signal canceled the run.
func execute() int {
	ctx, stop, exitStatus := shutdown.NotifyContext(context.Background(), nil)
	defer stop()

	err := rootCmd.ExecuteContext(ctx)

	return exitCodeFor(exitStatus(), err)
}

// exitCodeFor maps a command error to a process exit status. A graceful
// cancellation uses the signal-derived code (130 SIGINT / 143 SIGTERM); any
// other error uses 1.
func exitCodeFor(cancelCode int, err error) int {
	if err == nil {
		return 0
	}

	if errors.Is(err, context.Canceled) {
		return cancelCode
	}

	return 1
}

func Root() *cobra.Command {
	return rootCmd
}

func init() {
	cobra.EnableCommandSorting = false
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	rootCmd.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "%s" .Version}}`)

	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "output versions as JSON")
	rootCmd.AddCommand(versionCmd, run.Cmd, baseline.Cmd, probe.Cmd, help.Cmd)
}
