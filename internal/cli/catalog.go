package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/cmd/stroppy/commands/probe"
	runcommand "github.com/stroppy-io/stroppy/v6/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

var (
	errBuiltInNameCollision = errors.New("custom workload name conflicts with a built-in workload")
	errRemoveBuiltIn        = errors.New("cannot remove built-in workload")
	errOutputFormat         = errors.New("unsupported output format")
)

func newBuildCommand(catalog *bench.Catalog, store *workloadcatalog.Store) *cobra.Command {
	var replace bool

	command := &cobra.Command{
		Use:   "build [path]",
		Short: "Build and register a custom workload",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := "."
			if len(args) == 1 {
				source = args[0]
			}

			result, err := workloadcatalog.Build(cmd.Context(), source, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			defer result.Cleanup()

			if _, builtIn := catalog.Factory(result.Name); builtIn {
				return fmt.Errorf("%w: %s", errBuiltInNameCollision, result.Name)
			}

			entry, err := store.Publish(&workloadcatalog.Entry{
				Name: result.Name, Source: result.Source,
			}, result.Artifact, replace)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", entry.Name, entry.ArtifactPath)

			return err
		},
	}
	command.Flags().BoolVar(&replace, "replace", false, "replace an existing custom workload")

	return command
}

func newListCommand(catalog *bench.Catalog, store *workloadcatalog.Store) *cobra.Command {
	var format string

	command := &cobra.Command{
		Use:   "list",
		Short: "List built-in and custom workloads",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			builtIns, err := catalog.DescribeAll()
			if err != nil {
				return err
			}

			var custom []workloadcatalog.Entry
			if store != nil {
				custom, err = store.List()
				if err != nil {
					return err
				}
			}

			return writeList(cmd.OutOrStdout(), format, builtIns, custom)
		},
	}
	command.Flags().StringVarP(&format, "output", "o", "human", "output format: human or json")

	return command
}

func newRemoveCommand(catalog *bench.Catalog, store *workloadcatalog.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove a custom workload from the local catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, builtIn := catalog.Factory(args[0]); builtIn {
				return fmt.Errorf("%w %q", errRemoveBuiltIn, args[0])
			}

			if err := store.Remove(args[0]); err != nil {
				return err
			}

			_, err := fmt.Fprintln(cmd.OutOrStdout(), args[0])

			return err
		},
	}
}

func writeList(
	output io.Writer,
	format string,
	builtIns []bench.Description,
	custom []workloadcatalog.Entry,
) error {
	type item struct {
		Name     string `json:"name"`
		Origin   string `json:"origin"`
		Source   string `json:"source,omitempty"`
		Artifact string `json:"artifact,omitempty"`
	}

	items := make([]item, 0, len(builtIns)+len(custom))

	for _, description := range builtIns {
		items = append(items, item{Name: description.Name, Origin: "built-in"})
	}

	for _, entry := range custom {
		items = append(items, item{
			Name: entry.Name, Origin: "custom", Source: entry.Source, Artifact: entry.ArtifactPath,
		})
	}

	slices.SortFunc(items, func(left, right item) int { return strings.Compare(left.Name, right.Name) })

	switch format {
	case "json":
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")

		return encoder.Encode(items)
	case "human":
		for _, entry := range items {
			if _, err := fmt.Fprintf(output, "%-30s %s\n", entry.Name, entry.Origin); err != nil {
				return err
			}
		}

		return nil
	default:
		return fmt.Errorf("%w %q", errOutputFormat, format)
	}
}

func catalogResolver(store *workloadcatalog.Store) runcommand.Resolver {
	if store == nil {
		return nil
	}

	return managedResolver(store)
}

func addManagedCommands(
	root, probeCommand *cobra.Command,
	catalog *bench.Catalog,
	store *workloadcatalog.Store,
) {
	if store == nil {
		return
	}

	root.AddCommand(
		newBuildCommand(catalog, store),
		newListCommand(catalog, store),
		newRemoveCommand(catalog, store),
	)

	probeCommand.Args = cobra.ArbitraryArgs
	probeCommand.DisableFlagParsing = true
	probeCommand.RunE = managedProbe(catalog, store)
}

func managedResolver(store *workloadcatalog.Store) runcommand.Resolver {
	return func(ctx context.Context, command *cobra.Command, name string, args []string) (bool, error) {
		if _, err := store.Get(name); errors.Is(err, workloadcatalog.ErrNotFound) {
			return false, nil
		} else if err != nil {
			return false, err
		}

		return true, executeCustom(
			ctx, store, name, args,
			command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr(),
		)
	}
}

func managedProbe(catalog *bench.Catalog, store *workloadcatalog.Store) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			builtIn := probe.NewCommand(catalog)
			builtIn.SetContext(command.Context())
			builtIn.SetOut(command.OutOrStdout())
			builtIn.SetErr(command.ErrOrStderr())
			builtIn.SetArgs(args)

			return builtIn.Execute()
		}

		name := args[0]

		if _, err := store.Get(name); errors.Is(err, workloadcatalog.ErrNotFound) {
			return fmt.Errorf("%w: %s", workloadcatalog.ErrNotFound, name)
		} else if err != nil {
			return err
		}

		return executeCustom(
			command.Context(), store, name, append([]string{"probe"}, args[1:]...),
			command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr(),
		)
	}
}

func executeCustom(
	ctx context.Context,
	store *workloadcatalog.Store,
	name string,
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	return store.Run(ctx, name, args, &workloadcatalog.Process{
		Stdin: stdin, Stdout: stdout, Stderr: stderr, Env: os.Environ(), Dir: workingDirectory,
	})
}
