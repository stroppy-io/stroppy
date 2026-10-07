package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
)

var (
	errExportSelection = errors.New("select workload names or --all")
	errExportOutput    = errors.New("export output path is required")
	errMissingSource   = errors.New("workload source package is unavailable")
	errBrokenWorkload  = errors.New("workload is not ready")
	errDuplicateExport = errors.New("duplicate workload selection")
)

//nolint:gocognit // export validation and build sequence stays explicit
func newExportCommand(store *workloadcatalog.Store) *cobra.Command {
	var (
		all        bool
		output     string
		yes        bool
		offline    bool
		sourceRoot string
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
				return errExportOutput
			}

			entries, err := selectedEntries(store, args, all)
			if err != nil {
				return err
			}

			packages := make([]workloadcatalog.Package, 0, len(entries))
			for index := range entries {
				entry := &entries[index]
				if entry.SnapshotDigest != "" {
					pkg, err := store.Package(entry.SnapshotDigest)
					if err != nil {
						return err
					}

					packages = append(packages, pkg)

					continue
				}

				if entry.Package == "" || entry.ModuleRoot == "" {
					return fmt.Errorf("%w: %s", errMissingSource, entry.Name)
				}

				if _, err := os.Stat(entry.Source); err != nil {
					return fmt.Errorf("%w %s: %w", errMissingSource, entry.Name, err)
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

			absoluteOutput = targetOutputPath(absoluteOutput, targetOS)

			stroppyRoot, err := resolveSourceRoot(sourceRoot)
			if err != nil {
				return err
			}

			compiler, err := toolchain.Resolve(cmd.Context(), toolchain.Options{
				Root: store.StroppyRoot(), Consent: toolchainConsent(yes),
				Input: cmd.InOrStdin(), Output: cmd.ErrOrStderr(), Offline: offline,
			})
			if err != nil {
				return err
			}

			manifest, reused, err := workloadcatalog.BuildCached(
				cmd.Context(), compiler, &workloadcatalog.RunnerRequest{
					Packages: packages, IncludeBuiltIns: true, Output: absoluteOutput,
					TargetOS: targetOS, TargetArch: targetArch, Offline: offline,
					Diagnostics: cmd.ErrOrStderr(), StroppyRoot: stroppyRoot,
					CacheRoot: store.StroppyRoot(),
				},
			)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(
				cmd.OutOrStdout(), "%s\t%s/%s\tgo%s\t%d custom workloads\t%s\t%t\n",
				absoluteOutput, targetOS, targetArch, compiler.Version, len(packages),
				manifest.Digest, reused,
			)

			return err
		},
	}
	command.Flags().BoolVar(&all, "all", false, "include all custom catalog workloads")
	command.Flags().StringVarP(&output, "output", "o", "", "portable binary output path")
	command.Flags().StringVar(&sourceRoot, "source-root", "",
		"SDK source tree to compile against (default: the pinned module)")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "allow verified private Go download")
	command.Flags().BoolVar(&offline, "offline", false, "use only cached tools and modules")

	return command
}

func targetOutputPath(path, targetOS string) string {
	if targetOS == "windows" && !strings.EqualFold(filepath.Ext(path), ".exe") {
		return path + ".exe"
	}

	return path
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

		for index := range entries {
			entry := &entries[index]
			if entry.Status != "ready" {
				return nil, fmt.Errorf("%w: %s (%s)", errBrokenWorkload, entry.Name, entry.Status)
			}
		}

		return entries, nil
	}

	names = append([]string(nil), names...)
	slices.Sort(names)

	for index := 1; index < len(names); index++ {
		if names[index] == names[index-1] {
			return nil, fmt.Errorf("%w: %s", errDuplicateExport, names[index])
		}
	}

	entries := make([]workloadcatalog.Entry, 0, len(names))
	for _, name := range names {
		entry, err := store.Get(name)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
