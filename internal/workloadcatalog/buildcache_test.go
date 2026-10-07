package workloadcatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

func TestBuildCachedReusesCompletedArtifact(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	compiler, err := toolchain.Resolve(t.Context(), toolchain.Options{
		Root: t.TempDir(), Consent: toolchain.ConsentNever,
	})
	if err != nil {
		t.Fatal(err)
	}

	cacheRoot := t.TempDir()
	output := filepath.Join(t.TempDir(), "stroppy")
	request := &RunnerRequest{
		IncludeBuiltIns: true, Output: output, CacheRoot: cacheRoot,
		StroppyRoot: repoRoot, Diagnostics: &bytes.Buffer{},
	}

	first, reused, err := BuildCached(t.Context(), compiler, request)
	if err != nil || reused {
		t.Fatalf("first build = %#v reused=%t error=%v", first, reused, err)
	}

	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}

	second, reused, err := BuildCached(t.Context(), compiler, request)
	if err != nil || !reused {
		t.Fatalf("second build = %#v reused=%t error=%v", second, reused, err)
	}

	if first.Digest != second.Digest {
		t.Fatalf("digest changed: %s != %s", first.Digest, second.Digest)
	}
}

func TestBuildCachedRebuildsCorruptEntry(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	compiler, err := toolchain.Resolve(t.Context(), toolchain.Options{
		Root: t.TempDir(), Consent: toolchain.ConsentNever,
	})
	if err != nil {
		t.Fatal(err)
	}

	cacheRoot := t.TempDir()
	request := &RunnerRequest{
		IncludeBuiltIns: true, Output: filepath.Join(t.TempDir(), "stroppy"),
		CacheRoot: cacheRoot, StroppyRoot: repoRoot, Diagnostics: &bytes.Buffer{},
	}

	first, _, err := BuildCached(t.Context(), compiler, request)
	if err != nil {
		t.Fatal(err)
	}

	entry := filepath.Join(cacheRoot, "cache", "builds", first.Digest)
	if err := os.WriteFile(filepath.Join(entry, "stroppy"), []byte("corrupt"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(entry, "leftover"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	second, reused, err := BuildCached(t.Context(), compiler, request)
	if err != nil || reused {
		t.Fatalf("repair build = %#v reused=%t error=%v", second, reused, err)
	}

	if second.Digest != first.Digest {
		t.Fatalf("repair digest = %s, want %s", second.Digest, first.Digest)
	}

	if _, err := os.Stat(filepath.Join(entry, "leftover")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt entry was not replaced: %v", err)
	}
}

func TestRunnerSourcesReportTheMergedSDKVersion(t *testing.T) {
	cliVersion := stroppyModuleVersion()
	newer := "v6.99.99"

	for name, pinned := range map[string]string{
		"workload pinned older than the CLI": "v6.0.0",
		"workload pinned newer than the CLI": newer,
	} {
		t.Run(name, func(t *testing.T) {
			moduleRoot := t.TempDir()
			moduleData := fmt.Sprintf(
				"module example.com/input\n\ngo 1.27\n\nrequire %s %s\n",
				stroppyModulePath, pinned,
			)

			if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte(moduleData), 0o600); err != nil {
				t.Fatal(err)
			}

			pkg := Package{
				ImportPath: "example.com/input", Directory: moduleRoot,
				ModulePath: "example.com/input", ModuleRoot: moduleRoot,
			}

			_, moduleSource, effective, err := runnerSources([]Package{pkg}, true, "", "", "")
			if err != nil {
				t.Fatal(err)
			}

			want := cliVersion
			if semver.Compare(pinned, want) > 0 {
				want = pinned
			}

			if effective != want {
				t.Fatalf("effective SDK version = %q, want %q", effective, want)
			}

			// The generated go.mod is the authority: the reported version must be
			// the one the artifact is actually built against.
			require.Contains(t, string(moduleSource), stroppyModulePath+" "+want)
		})
	}
}

func TestRunnerIdentityRecordsTheMergedSDKVersion(t *testing.T) {
	moduleRoot := t.TempDir()
	pinned := "v6.99.99"
	moduleData := fmt.Sprintf(
		"module example.com/input\n\ngo 1.27\n\nrequire %s %s\n", stroppyModulePath, pinned,
	)

	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte(moduleData), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(moduleRoot, "workload.go"), []byte("package input\nconst Value = 1\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	compiler, err := toolchain.Resolve(t.Context(), toolchain.Options{
		Root: t.TempDir(), Consent: toolchain.ConsentNever,
	})
	if err != nil {
		t.Fatal(err)
	}

	pkg := Package{
		ImportPath: "example.com/input", Directory: moduleRoot,
		ModulePath: "example.com/input", ModuleRoot: moduleRoot,
	}

	identity, err := runnerIdentity(compiler, &RunnerRequest{Packages: []Package{pkg}, IncludeBuiltIns: true})
	if err != nil {
		t.Fatal(err)
	}

	if identity.StroppyModuleVersion != pinned {
		t.Fatalf("identity SDK module version = %q, want the merged %q",
			identity.StroppyModuleVersion, pinned)
	}

	// The requested release version stays the runner's own identity.
	if identity.StroppyVersion == pinned {
		t.Fatalf("requested version = %q; it must not be replaced by the merged pin", identity.StroppyVersion)
	}
}

func TestBuildCachedInvalidatesUnsnappedSource(t *testing.T) {
	moduleRoot := t.TempDir()

	moduleData := []byte("module example.com/input\n\ngo 1.27\n")
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), moduleData, 0o600); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(moduleRoot, "workload.go")
	if err := os.WriteFile(source, []byte("package input\nconst Value = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	compiler, err := toolchain.Resolve(t.Context(), toolchain.Options{
		Root: t.TempDir(), Consent: toolchain.ConsentNever,
	})
	if err != nil {
		t.Fatal(err)
	}

	pkg, err := DiscoverPackage(t.Context(), compiler, moduleRoot, false)
	if err != nil {
		t.Fatal(err)
	}

	request := &RunnerRequest{Packages: []Package{pkg}, IncludeBuiltIns: true}

	first, err := runnerIdentity(compiler, request)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(source, []byte("package input\nconst Value = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	second, err := runnerIdentity(compiler, request)
	if err != nil {
		t.Fatal(err)
	}

	firstDigest, err := canonicalDigest(first)
	if err != nil {
		t.Fatal(err)
	}

	secondDigest, err := canonicalDigest(second)
	if err != nil {
		t.Fatal(err)
	}

	if firstDigest == secondDigest {
		t.Fatal("source edit did not invalidate build identity")
	}
}

func TestBuildCachedConcurrentWritersShareArtifact(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	compiler, err := toolchain.Resolve(t.Context(), toolchain.Options{
		Root: t.TempDir(), Consent: toolchain.ConsentNever,
	})
	if err != nil {
		t.Fatal(err)
	}

	cacheRoot := t.TempDir()
	start := make(chan struct{})
	results := make(chan struct {
		manifest BuildManifest
		reused   bool
		err      error
	}, 2)

	outputs := []string{
		filepath.Join(t.TempDir(), "stroppy"),
		filepath.Join(t.TempDir(), "stroppy"),
	}

	var wait sync.WaitGroup
	for index := range 2 {
		wait.Go(func() {
			<-start

			manifest, reused, err := BuildCached(t.Context(), compiler, &RunnerRequest{
				IncludeBuiltIns: true, Output: outputs[index],
				CacheRoot: cacheRoot, StroppyRoot: repoRoot, Diagnostics: &bytes.Buffer{},
			})
			results <- struct {
				manifest BuildManifest
				reused   bool
				err      error
			}{manifest, reused, err}
		})
	}

	close(start)
	wait.Wait()
	close(results)

	digest := ""
	reusedCount := 0

	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}

		if digest != "" && result.manifest.Digest != digest {
			t.Fatalf("concurrent digests differ: %s != %s", digest, result.manifest.Digest)
		}

		digest = result.manifest.Digest
		if result.reused {
			reusedCount++
		}
	}

	if reusedCount != 1 {
		t.Fatalf("reused count = %d, want 1", reusedCount)
	}
}

func TestInspectBuildUsesUniquePrefix(t *testing.T) {
	root := t.TempDir()

	cache, err := OpenCache(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, digest := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		directory := filepath.Join(cache.root, digest)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}

		artifact := filepath.Join(directory, "stroppy")
		if err := os.WriteFile(artifact, []byte(digest), 0o700); err != nil {
			t.Fatal(err)
		}

		checksum, err := digestFile(artifact)
		if err != nil {
			t.Fatal(err)
		}

		manifest := BuildManifest{Schema: BuildSchemaVersion, Digest: digest, ArtifactSHA256: checksum}

		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	manifest, err := cache.Inspect("aaaa")
	if err != nil || manifest.Digest != strings.Repeat("a", 64) {
		t.Fatalf("inspect unique prefix = %#v, %v", manifest, err)
	}

	if _, err := cache.Inspect(""); !errors.Is(err, ErrBuildAmbiguous) {
		t.Fatalf("inspect ambiguous prefix = %v", err)
	}
}
