package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/help"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/probe"
	"github.com/stroppy-io/stroppy/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/internal/version"
	_ "github.com/stroppy-io/stroppy/internal/workloads"
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

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func Root() *cobra.Command {
	return rootCmd
}

func init() {
	cobra.EnableCommandSorting = false
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	rootCmd.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "%s" .Version}}`)

	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "output versions as JSON")
	rootCmd.AddCommand(versionCmd, run.Cmd, probe.Cmd, help.Cmd)
}
