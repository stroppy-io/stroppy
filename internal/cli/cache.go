package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
)

var errCacheOutputFormat = errors.New("unsupported cache output format")

func newCacheCommand(store *workloadcatalog.Store) *cobra.Command {
	command := &cobra.Command{Use: "cache", Short: "Inspect or clean Stroppy build cache"}
	command.AddCommand(newCacheInspectCommand(store), newCacheCleanCommand(store))

	return command
}

func newCacheInspectCommand(store *workloadcatalog.Store) *cobra.Command {
	var format string

	command := &cobra.Command{
		Use:   "inspect DIGEST",
		Short: "Show build provenance by full or unique digest prefix",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, err := workloadcatalog.InspectBuild(store.StroppyRoot(), args[0])
			if errors.Is(err, workloadcatalog.ErrBuildNotFound) {
				if active, activeErr := store.ActiveRuntime(); activeErr == nil &&
					len(args[0]) <= len(active.BuildDigest) && active.BuildDigest[:len(args[0])] == args[0] {
					manifest, err = workloadcatalog.InspectBuild(store.StroppyRoot(), active.BuildDigest)
				}
			}

			if err != nil {
				return err
			}

			switch format {
			case "json":
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")

				return encoder.Encode(manifest)
			case "human":
				_, err = fmt.Fprintf(
					cmd.OutOrStdout(), "%s\t%s/%s\tgo%s\t%d workloads\tsdk=%s\n",
					manifest.Digest, manifest.TargetOS, manifest.TargetArch,
					manifest.GoVersion, len(manifest.Workloads), buildSDKOrigin(store, &manifest),
				)

				return err
			default:
				return fmt.Errorf("%w %q", errCacheOutputFormat, format)
			}
		},
	}
	command.Flags().StringVarP(&format, "output", "o", "human", "output format: human or json")

	return command
}

func newCacheCleanCommand(store *workloadcatalog.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove reusable build artifacts and private Go caches",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := errors.Join(
				workloadcatalog.CleanBuildCache(store.StroppyRoot()),
				toolchain.CleanPrivateCaches(store.StroppyRoot()),
			); err != nil {
				return err
			}

			_, err := fmt.Fprintln(cmd.OutOrStdout(), "build cache cleaned")

			return err
		},
	}
}

// buildSDKOrigin names the SDK a cached artifact embeds: the exact source tree
// when it is the active runtime's, otherwise the recorded source hash or the
// pinned module.
func buildSDKOrigin(store *workloadcatalog.Store, manifest *workloadcatalog.BuildManifest) string {
	if active, err := store.ActiveRuntime(); err == nil && active.BuildDigest == manifest.Digest {
		if active.StroppySource != "" {
			return "source tree " + active.StroppySource
		}

		return "module " + active.StroppyVersion
	}

	if manifest.StroppySource != "" {
		return "source hash " + shortDigest(manifest.StroppySource)
	}

	return "module " + manifest.StroppyVersion
}
