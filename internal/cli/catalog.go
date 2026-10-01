package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/cmd/stroppy/commands/probe"
	runcommand "github.com/stroppy-io/stroppy/v6/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
	"github.com/stroppy-io/stroppy/v6/internal/version"
	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

var (
	errBuiltInNameCollision = errors.New("custom workload name conflicts with a built-in workload")
	errRemoveBuiltIn        = errors.New("cannot remove built-in workload")
	errOutputFormat         = errors.New("unsupported output format")
	errRefreshSource        = errors.New("--refresh does not accept a source path")
)

//nolint:gocognit // build transaction keeps snapshot, catalog, runtime, and rollback order explicit
func newBuildCommand(catalog *bench.Catalog, store *workloadcatalog.Store) *cobra.Command {
	var (
		replace bool
		refresh bool
		yes     bool
		offline bool
	)

	command := &cobra.Command{
		Use:   "build [path]",
		Short: "Build and register a custom workload",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if refresh && len(args) != 0 {
				return errRefreshSource
			}

			compiler, err := toolchain.Resolve(cmd.Context(), toolchain.Options{
				Root: store.StroppyRoot(), Consent: toolchainConsent(yes),
				Input: cmd.InOrStdin(), Output: cmd.ErrOrStderr(), Offline: offline,
			})
			if err != nil {
				return err
			}

			if refresh {
				active, reused, err := store.RebuildRuntime(
					cmd.Context(), compiler, cmd.ErrOrStderr(), offline,
					stroppySourceRoot(), version.Resolve(),
				)
				if err != nil {
					return err
				}

				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%t\n", active.BuildDigest, reused)

				return err
			}

			source := "."
			if len(args) == 1 {
				source = args[0]
			}

			result, snapshot, err := store.BuildSnapshot(
				cmd.Context(), compiler, source, cmd.ErrOrStderr(), offline, stroppySourceRoot(),
			)
			if err != nil {
				return err
			}
			defer result.Cleanup()

			if _, builtIn := catalog.Factory(result.Name); builtIn {
				return fmt.Errorf("%w: %s", errBuiltInNameCollision, result.Name)
			}

			candidate := &workloadcatalog.Entry{
				Name: result.Name, Source: result.Source,
				Package: result.Package.ImportPath, ModulePath: result.Package.ModulePath,
				ModuleRoot: result.Package.ModuleRoot, SnapshotDigest: snapshot.Digest,
			}

			previous, previousErr := store.Get(result.Name)
			if previousErr != nil && !errors.Is(previousErr, workloadcatalog.ErrNotFound) {
				return previousErr
			}

			entry, err := store.Publish(candidate, "", replace)
			if err != nil {
				return err
			}

			active, reused, err := store.RebuildRuntime(
				cmd.Context(), compiler, cmd.ErrOrStderr(), offline,
				stroppySourceRoot(), version.Resolve(),
			)
			if err != nil {
				removeErr := store.Remove(result.Name)

				var restoreErr error
				if previousErr == nil {
					restoreErr = store.Restore(&previous)
				}

				return errors.Join(err, removeErr, restoreErr)
			}

			_, err = fmt.Fprintf(
				cmd.OutOrStdout(), "%s\t%s\t%s\t%t\n",
				entry.Name, snapshot.Digest, active.BuildDigest, reused,
			)

			return err
		},
	}
	command.Flags().BoolVar(&replace, "replace", false, "replace an existing custom workload")
	command.Flags().BoolVar(&refresh, "refresh", false, "rebuild local runtime from catalog snapshots")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "allow verified private Go download")
	command.Flags().BoolVar(&offline, "offline", false, "use only cached tools and modules")

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
	var (
		yes     bool
		offline bool
	)

	command := &cobra.Command{
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

			packages, err := store.Packages()
			if err != nil {
				return err
			}

			if len(packages) == 0 {
				if err := store.RemoveRuntime(); err != nil {
					return err
				}
			} else {
				compiler, err := toolchain.Resolve(cmd.Context(), toolchain.Options{
					Root: store.StroppyRoot(), Consent: toolchainConsent(yes),
					Input: cmd.InOrStdin(), Output: cmd.ErrOrStderr(), Offline: offline,
				})
				if err != nil {
					return err
				}

				if _, _, err := store.RebuildRuntime(
					cmd.Context(), compiler, cmd.ErrOrStderr(), offline,
					stroppySourceRoot(), version.Resolve(),
				); err != nil {
					return err
				}
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), args[0])

			return err
		},
	}
	command.Flags().BoolVarP(&yes, "yes", "y", false, "allow verified private Go download")
	command.Flags().BoolVar(&offline, "offline", false, "use only cached tools and modules")

	return command
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

	for index := range custom {
		entry := &custom[index]
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
		newCacheCommand(store),
		newExportCommand(store),
		newListCommand(catalog, store),
		newRemoveCommand(catalog, store),
	)

	probeCommand.Args = cobra.ArbitraryArgs
	probeCommand.DisableFlagParsing = true
	probeCommand.RunE = managedProbe(catalog, store)
}

func managedResolver(store *workloadcatalog.Store) runcommand.Resolver {
	return func(ctx context.Context, command *cobra.Command, name string, args []string) (bool, error) {
		entry, err := store.Get(name)
		if errors.Is(err, workloadcatalog.ErrNotFound) {
			return false, nil
		}

		if err != nil {
			return false, err
		}

		if entry.SnapshotDigest == "" {
			return true, executeCustom(
				ctx, store, name, args,
				command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr(),
			)
		}

		return true, executeRuntime(command, store, append([]string{"run", name}, args...))
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

		entry, err := store.Get(name)
		if errors.Is(err, workloadcatalog.ErrNotFound) {
			return fmt.Errorf("%w: %s", workloadcatalog.ErrNotFound, name)
		}

		if err != nil {
			return err
		}

		if entry.SnapshotDigest != "" {
			return executeRuntime(command, store, append([]string{"probe"}, args[1:]...))
		}

		return executeCustom(
			command.Context(), store, name, append([]string{"probe"}, args[1:]...),
			command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr(),
		)
	}
}

func toolchainConsent(yes bool) toolchain.Consent {
	if yes {
		return toolchain.ConsentAlways
	}

	return toolchain.ConsentAsk
}

func stroppySourceRoot() string {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	root, err := moduleRoot(filepath.Dir(source))
	if err != nil {
		return ""
	}

	return root
}

func moduleRoot(start string) (string, error) {
	path, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
			return path, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			return "", os.ErrNotExist
		}

		path = parent
	}
}

func executeRuntime(
	command *cobra.Command,
	store *workloadcatalog.Store,
	args []string,
) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	return store.RunRuntime(command.Context(), version.Resolve(), args, &workloadcatalog.RuntimeProcess{
		Stdin: command.InOrStdin(), Stdout: command.OutOrStdout(), Stderr: command.ErrOrStderr(),
		Env: os.Environ(), Dir: workingDirectory,
	})
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
