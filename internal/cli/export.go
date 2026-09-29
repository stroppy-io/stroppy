package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
)

var (
	errExportSelection = errors.New("select workload names or --all")
	errMissingSource   = errors.New("workload source package is unavailable")
)

func newExportCommand(store *workloadcatalog.Store) *cobra.Command {
	var (
		all     bool
		output  string
		yes     bool
		offline bool
	)

	command := &cobra.Command{
		Use:   "export [workload ...]",
		Short: "Build a portable Stroppy binary",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return errExportSelection
			}
			if output == "" {
				return errors.New("export output path is required")
			}

			compiler, err := toolchain.Resolve(cmd.Context(), toolchain.Options{
				Root: store.StroppyRoot(), Consent: toolchainConsent(yes),
				Input: cmd.InOrStdin(), Output: cmd.ErrOrStderr(), Offline: offline,
			})
			if err != nil {
				return err
			}

			entries, err := selectedEntries(store, args, all)
			if err != nil {
				return err
			}

			packages := make([]workloadcatalog.Package, 0, len(entries))
			for _, entry := range entries {
				if entry.Package == "" || entry.ModuleRoot == "" {
					return fmt.Errorf("%w: %s", errMissingSource, entry.Name)
				}
				if _, err := os.Stat(entry.Source); err != nil {
					return fmt.Errorf("%w %s: %v", errMissingSource, entry.Name, err)
				}

				packages = append(packages, workloadcatalog.Package{
					ImportPath: entry.Package, Directory: entry.Source,
					ModulePath: entry.ModulePath, ModuleRoot: entry.ModuleRoot,
				})
			}

			targetOS := os.Getenv("GOOS")
			if targetOS == "" {
				targetOS = runtime.GOOS
			}
			targetArch := os.Getenv("GOARCH")
			if targetArch == "" {
				targetArch = runtime.GOARCH
			}

			absoluteOutput, err := filepath.Abs(output)
			if err != nil {
				return err
			}

			if err := workloadcatalog.BuildRunner(cmd.Context(), compiler, workloadcatalog.RunnerRequest{
				Packages: packages, IncludeBuiltIns: true, Output: absoluteOutput,
				TargetOS: targetOS, TargetArch: targetArch, Offline: offline,
				Diagnostics: cmd.ErrOrStderr(), StroppyRoot: stroppySourceRoot(),
			}); err != nil {
				return err
			}

			_, err = fmt.Fprintf(
				cmd.OutOrStdout(), "%s\t%s/%s\tgo%s\t%d custom workloads\n",
				absoluteOutput, targetOS, targetArch, compiler.Version, len(packages),
			)

			return err
		},
	}
	command.Flags().BoolVar(&all, "all", false, "include all custom catalog workloads")
	command.Flags().StringVarP(&output, "output", "o", "", "portable binary output path")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "allow verified private Go download")
	command.Flags().BoolVar(&offline, "offline", false, "use only cached tools and modules")

	return command
}

func selectedEntries(
	store *workloadcatalog.Store,
	names []string,
	all bool,
) ([]workloadcatalog.Entry, error) {
	if all {
		entries, err := store.List()
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Status != "ready" {
				return nil, fmt.Errorf("workload %q is %s", entry.Name, entry.Status)
			}
		}

		return entries, nil
	}

	names = append([]string(nil), names...)
	slices.Sort(names)
	entries := make([]workloadcatalog.Entry, 0, len(names))
	for index, name := range names {
		if index > 0 && name == names[index-1] {
			return nil, fmt.Errorf("duplicate workload %q", name)
		}

		entry, err := store.Get(name)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}
