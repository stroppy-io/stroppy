package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/internal/workloadcatalog"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func TestCacheInspectDoesNotHideAmbiguousPrefix(t *testing.T) {
	root := t.TempDir()

	store, err := workloadcatalog.OpenAt(filepath.Join(root, "workloads"))
	if err != nil {
		t.Fatal(err)
	}

	for _, digest := range []string{strings.Repeat("a", 64), "a" + strings.Repeat("b", 63)} {
		directory := filepath.Join(root, "cache", "builds", digest)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}

		artifact := filepath.Join(directory, "stroppy")
		if err := os.WriteFile(artifact, []byte(digest), 0o700); err != nil {
			t.Fatal(err)
		}

		checksum, err := fileSHA256(artifact)
		if err != nil {
			t.Fatal(err)
		}

		manifest := workloadcatalog.BuildManifest{
			Schema: workloadcatalog.BuildSchemaVersion, Digest: digest, ArtifactSHA256: checksum,
		}

		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer

	err = Execute(t.Context(), &Options{
		Catalog: bench.RegisteredCatalog(), ManagedCatalog: store,
	}, []string{"cache", "inspect", "a"}, &stdout, &stderr)
	if !errors.Is(err, workloadcatalog.ErrBuildAmbiguous) {
		t.Fatalf("cache inspect error = %v, want ErrBuildAmbiguous", err)
	}
}
