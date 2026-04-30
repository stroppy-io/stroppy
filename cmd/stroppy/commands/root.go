package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/spf13/cobra"
	"go.k6.io/k6/cmd/state"

	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/gen"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/help"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/probe"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/internal/runner"
	"github.com/stroppy-io/stroppy/internal/version"
)

var rootCmd = &cobra.Command{
	Use:   "stroppy",
	Short: "Tool to generate and run stress tests (e.g benchmarking) for databases",
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
			"stroppy": version.Version,
		}

		// Pull dependency versions from the compiled binary's module info.
		// These stay in sync with go.mod automatically — no hardcoding.
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, dep := range info.Deps {
				switch dep.Path {
				case "go.k6.io/k6":
					versions["k6"] = dep.Version
				case "github.com/jackc/pgx/v5":
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
				{"stroppy", versions["stroppy"]},
				{"k6", versions["k6"]},
				{"pgx", versions["pgx"]},
			} {
				if kv.v != "" {
					fmt.Fprintf(os.Stdout, "%-8s %s\n", kv.k, kv.v)
				}
			}
		}
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func Root() *cobra.Command {
	return rootCmd
}

func K6Subcommand(gs *state.GlobalState) *cobra.Command {
	inteceptInteruptSignals(gs)
	if runner.K6ExitCaptureEnabled() {
		gs.OSExit = runner.OSExit
	}

	return rootCmd
}

func init() {
	// Skip "k6 x stroppy" prefix if binary file already named as "stroppy"
	if filepath.Base(os.Args[0]) == "stroppy" {
		os.Args = append([]string{"k6", "x", "stroppy"}, os.Args[1:]...)

		// [cobra.Command] help message should think that stroppy rootCmd have no parent
		oldUsageFunc := rootCmd.UsageFunc()
		rootCmd.SetUsageFunc(func(c *cobra.Command) error {
			parent := rootCmd.Parent()
			parent.RemoveCommand(rootCmd)

			err := oldUsageFunc(c)

			parent.AddCommand(rootCmd)

			return err
		})
	}

	cobra.EnableCommandSorting = false
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	rootCmd.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "%s" .Version}}`)

	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "output versions as JSON")
	rootCmd.AddCommand(versionCmd, run.Cmd, gen.Cmd, probe.Cmd, help.Cmd)
}
