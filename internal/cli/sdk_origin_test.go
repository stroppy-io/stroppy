package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
)

// A runtime built from the pinned module must report the version the generated
// go.mod actually requires, which can be raised by a workload's own pin.
func TestReportRuntimeSourceNamesTheEmbeddedSDK(t *testing.T) {
	for name, expected := range map[string]struct {
		active workloadcatalog.ActiveRuntime
		want   string
	}{
		"merged module version": {
			active: workloadcatalog.ActiveRuntime{
				BuildDigest:          strings.Repeat("a", 64),
				StroppyVersion:       "v6.1.1",
				StroppyModuleVersion: "v6.99.99",
			},
			want: "sdk module github.com/stroppy-io/stroppy/v6 v6.99.99",
		},
		"version without merged provenance falls back": {
			active: workloadcatalog.ActiveRuntime{
				BuildDigest:    strings.Repeat("b", 64),
				StroppyVersion: "v6.1.1",
			},
			want: "sdk module github.com/stroppy-io/stroppy/v6 v6.1.1",
		},
		"source tree wins over the module": {
			active: workloadcatalog.ActiveRuntime{
				BuildDigest:          strings.Repeat("c", 64),
				StroppyVersion:       "v6.1.1",
				StroppyModuleVersion: "v6.99.99",
				StroppySource:        "/src/stroppy",
			},
			want: "sdk source tree /src/stroppy",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer

			active := expected.active

			command := &cobra.Command{}
			command.SetErr(&stderr)

			reportRuntimeSource(command, &active)

			if !strings.Contains(stderr.String(), expected.want) {
				t.Fatalf("reported %q, want it to contain %q", stderr.String(), expected.want)
			}
		})
	}
}

func TestBuildSDKOriginUsesTheManifestVersion(t *testing.T) {
	store, err := workloadcatalog.OpenAt(filepath.Join(t.TempDir(), "catalog"))
	if err != nil {
		t.Fatal(err)
	}

	manifest := workloadcatalog.BuildManifest{
		Digest:               strings.Repeat("d", 64),
		StroppyVersion:       "v6.1.1",
		StroppyModuleVersion: "v6.99.99",
	}

	if got := buildSDKOrigin(store, &manifest); got != "module v6.99.99" {
		t.Fatalf("buildSDKOrigin = %q, want the merged module version", got)
	}

	manifest.StroppyModuleVersion = ""

	if got := buildSDKOrigin(store, &manifest); got != "module v6.1.1" {
		t.Fatalf("buildSDKOrigin without merged provenance = %q, want the requested version", got)
	}

	manifest.StroppySource = strings.Repeat("e", 64)

	if got := buildSDKOrigin(store, &manifest); !strings.HasPrefix(got, "source hash ") {
		t.Fatalf("buildSDKOrigin with a source tree = %q", got)
	}
}
