// Package cli assembles Stroppy's reusable command interface.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/cmd/stroppy/commands/help"
	"github.com/stroppy-io/stroppy/v6/cmd/stroppy/commands/probe"
	runcommand "github.com/stroppy-io/stroppy/v6/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/v6/internal/version"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

const appName = "stroppy"

// Options selects installed or standalone command behavior.
type Options struct {
	Catalog         *bench.Catalog
	DefaultWorkload string
	ExtraCommands   []*cobra.Command
}

// NewRoot creates an independent Stroppy command tree.
func NewRoot(options Options) *cobra.Command {
	cobra.EnableCommandSorting = false

	catalog := options.Catalog
	if catalog == nil {
		catalog = bench.RegisteredCatalog()
	}

	root := &cobra.Command{
		Use:                appName,
		Short:              "Generate and run Go-native database stress tests",
		SilenceUsage:       true,
		DisableFlagParsing: options.DefaultWorkload != "",
	}
	root.CompletionOptions.HiddenDefaultCmd = true
	root.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "%s" .Version}}`)

	run := runcommand.NewCommand(catalog, options.DefaultWorkload)
	root.AddCommand(newVersionCommand(), probe.NewCommand(catalog), help.NewCommand())
	root.AddCommand(options.ExtraCommands...)

	if options.DefaultWorkload == "" {
		root.AddCommand(run)
	} else {
		root.Args = cobra.ArbitraryArgs
		root.RunE = func(cmd *cobra.Command, args []string) error {
			run.SetContext(cmd.Context())
			run.SetOut(cmd.OutOrStdout())
			run.SetErr(cmd.ErrOrStderr())

			return run.RunE(run, args)
		}
		root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
			run.SetContext(cmd.Context())
			run.SetOut(cmd.OutOrStdout())
			run.SetErr(cmd.ErrOrStderr())
			_ = run.RunE(run, []string{"--help"})
		})
	}

	return root
}

// Execute runs one command tree with caller-owned context and streams.
func Execute(
	ctx context.Context,
	options Options,
	args []string,
	stdout, stderr io.Writer,
) error {
	root := NewRoot(options)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	return root.ExecuteContext(ctx)
}

// ExitCodeFor maps a command error to its process exit status.
func ExitCodeFor(cancelCode int, err error) int {
	if err == nil {
		return 0
	}

	if errors.Is(err, context.Canceled) {
		return cancelCode
	}

	return 1
}

func newVersionCommand() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print versions of stroppy components",
		RunE: func(cmd *cobra.Command, _ []string) error {
			versions := componentVersions()
			if jsonOutput {
				versions["ydb_service_account_key_file"] = "1"
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")

				return encoder.Encode(versions)
			}

			for _, item := range []struct{ name, value string }{
				{appName, versions[appName]},
				{"pgx", versions["pgx"]},
			} {
				if item.value != "" {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-8s %s\n", item.name, item.value); err != nil {
						return err
					}
				}
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output versions as JSON")

	return cmd
}

func componentVersions() map[string]string {
	versions := map[string]string{appName: version.Resolve()}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dependency := range info.Deps {
			if dependency.Path == "github.com/jackc/pgx/v5" {
				versions["pgx"] = dependency.Version
			}
		}
	}

	return versions
}
