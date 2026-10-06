package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"

	"github.com/stroppy-io/stroppy/v6/internal/author"
	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
	"github.com/stroppy-io/stroppy/v6/internal/version"
	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

const ejectArgumentCount = 2

var errSDKVersion = errors.New("compatible SDK version required; pass --sdk-version with a release or pseudo-version")

func newInitCommand() *cobra.Command {
	var (
		modulePath, sdkVersion string
		yes, offline           bool
	)

	command := &cobra.Command{
		Use: "init PATH", Short: "Create a minimal standalone workload project", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := filepath.Base(filepath.Clean(args[0]))
			if modulePath == "" {
				modulePath = author.ModuleName(name)
			}

			selected, err := authorVersion(sdkVersion)
			if err != nil {
				return err
			}

			files, err := author.Project(name, modulePath, selected, author.Starter(name))
			if err != nil {
				return err
			}

			if err := author.Write(args[0], files); err != nil {
				return err
			}

			if err := resolveProject(cmd, args[0], yes, offline); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created %s. Run 'go run .' and 'go test ./...' inside it.\n", args[0])

			return err
		},
	}
	command.Flags().StringVar(&modulePath, "module", "", "Go module path (default example.com/<name>)")
	command.Flags().StringVar(&sdkVersion, "sdk-version", "", "Stroppy release or pseudo-version")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "allow verified private Go download")
	command.Flags().BoolVar(&offline, "offline", false, "resolve only cached dependencies")

	return command
}

//nolint:gocognit,nestif // managed delegation and local source restoration share command semantics.
func newEjectCommand(catalog *bench.Catalog, store *workloadcatalog.Store) *cobra.Command {
	var (
		modulePath, sdkVersion string
		yes, offline           bool
	)

	command := &cobra.Command{
		Use: "eject NAME PATH", Short: "Restore explicitly published workload source",
		Args: cobra.ExactArgs(ejectArgumentCount),
		RunE: func(cmd *cobra.Command, args []string) error {
			test, ok := catalog.Test(args[0])
			if !ok && store != nil {
				entry, err := store.Get(args[0])
				if err != nil {
					return err
				}

				forward := append([]string{"eject"}, args...)
				if modulePath != "" {
					forward = append(forward, "--module", modulePath)
				}

				if sdkVersion != "" {
					forward = append(forward, "--sdk-version", sdkVersion)
				}

				if yes {
					forward = append(forward, "-y")
				}

				if offline {
					forward = append(forward, "--offline")
				}

				if entry.SnapshotDigest != "" {
					return executeRuntime(cmd, store, forward)
				}

				return executeCustom(cmd.Context(), store, args[0], forward, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			}

			if !ok {
				return fmt.Errorf("%w: %s", workloadcatalog.ErrNotFound, args[0])
			}

			published, err := author.Read(test.Source)
			if err != nil {
				return fmt.Errorf("workload %q: %w", test.Name, err)
			}

			selected, err := authorVersion(sdkVersion)
			if err != nil {
				return err
			}

			if modulePath == "" {
				modulePath = author.ModuleName(filepath.Base(filepath.Clean(args[1])))
			}

			files, err := author.Project(test.Name, modulePath, selected, published)
			if err != nil {
				return err
			}

			if err := author.Write(args[1], files); err != nil {
				return err
			}

			if err := resolveProject(cmd, args[1], yes, offline); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Restored %s to %s\n", test.Name, args[1])

			return err
		},
	}
	command.Flags().StringVar(&modulePath, "module", "", "Go module path for the restored project")
	command.Flags().StringVar(&sdkVersion, "sdk-version", "", "Stroppy release or pseudo-version")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "allow verified private Go download")
	command.Flags().BoolVar(&offline, "offline", false, "resolve only cached dependencies")

	return command
}

//nolint:nestif // explicit version overrides linked-module and release-build identity.
func authorVersion(explicit string) (string, error) {
	selected := explicit
	if selected == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Path == author.SDK {
				selected = info.Main.Version
			}

			for _, dependency := range info.Deps {
				if dependency.Path == author.SDK {
					selected = dependency.Version

					break
				}
			}
		}

		if !semver.IsValid(selected) {
			selected = version.Resolve()
		}
	}

	if !semver.IsValid(selected) || !strings.HasPrefix(selected, "v6.") ||
		strings.Contains(selected, "-g") {
		return "", errSDKVersion
	}

	return selected, nil
}

func resolveProject(cmd *cobra.Command, directory string, yes, offline bool) error {
	compiler, err := toolchain.Resolve(cmd.Context(), toolchain.Options{
		Consent: toolchainConsent(yes), Offline: offline, Input: cmd.InOrStdin(), Output: cmd.ErrOrStderr(),
	})
	if err != nil {
		return err
	}

	//nolint:gosec // compiler is resolved and verified; arguments are fixed project dependency inspection.
	command := exec.CommandContext(cmd.Context(), compiler.Path, "list", "-mod=mod", "-deps", "-test", "./...")
	command.Dir = directory
	command.Env = compiler.Env("", "", offline)

	var diagnostics bytes.Buffer

	command.Stdout = &diagnostics

	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		return fmt.Errorf("project created, dependency resolution failed "+
			"(retry go list -mod=mod -deps -test ./...): %w: %s", err, diagnostics.String())
	}

	return nil
}
