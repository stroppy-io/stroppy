package workloadcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

const runtimeSchemaVersion = 1

var (
	ErrRuntimeNotFound = errors.New("local Stroppy runtime not found")
	ErrRuntimeStale    = errors.New("local Stroppy runtime is stale; run 'stroppy build --refresh'")
)

// ActiveRuntime identifies one published local Stroppy executable.
type ActiveRuntime struct {
	Schema         int       `json:"schema"`
	BuildDigest    string    `json:"build_digest"`
	StroppyVersion string    `json:"stroppy_version"`
	CatalogDigest  string    `json:"catalog_digest"`
	ArtifactPath   string    `json:"artifact_path"`
	BuiltAt        time.Time `json:"built_at"`
}

// RuntimeProcess describes direct argument and stream forwarding to local Stroppy.
type RuntimeProcess struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    []string
	Dir    string
}

// Packages returns every catalog workload from its owned snapshot.
func (store *Store) Packages() ([]Package, error) {
	entries, err := store.List()
	if err != nil {
		return nil, err
	}

	packages := make([]Package, 0, len(entries))
	for index := range entries {
		entry := &entries[index]
		if entry.SnapshotDigest == "" {
			continue
		}

		snapshot, err := store.readSnapshot(entry.SnapshotDigest)
		if err != nil {
			return nil, err
		}

		pkg, err := store.snapshotPackage(&snapshot)
		if err != nil {
			return nil, err
		}

		packages = append(packages, pkg)
	}

	sort.Slice(packages, func(left, right int) bool {
		return packages[left].ImportPath < packages[right].ImportPath
	})

	return packages, nil
}

// RebuildRuntime builds and atomically activates Stroppy with every catalog workload.
func (store *Store) RebuildRuntime(
	ctx context.Context,
	compiler *toolchain.Compiler,
	diagnostics io.Writer,
	offline bool,
	stroppyRoot, buildVersion string,
) (ActiveRuntime, bool, error) {
	packages, err := store.Packages()
	if err != nil {
		return ActiveRuntime{}, false, err
	}

	if len(packages) == 0 {
		if err := store.RemoveRuntime(); err != nil {
			return ActiveRuntime{}, false, err
		}

		return ActiveRuntime{}, false, nil
	}

	catalogDigest, err := packageCatalogDigest(packages)
	if err != nil {
		return ActiveRuntime{}, false, err
	}

	runtimeDir := filepath.Join(store.StroppyRoot(), "runtime")

	staging, err := os.MkdirTemp(runtimeDir, ".runtime-*")
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(runtimeDir, dirPerm); err != nil {
			return ActiveRuntime{}, false, err
		}

		staging, err = os.MkdirTemp(runtimeDir, ".runtime-*")
	}

	if err != nil {
		return ActiveRuntime{}, false, err
	}

	defer os.RemoveAll(staging)

	artifact := filepath.Join(staging, "stroppy")

	manifest, reused, err := BuildCached(ctx, compiler, &RunnerRequest{
		Packages: packages, IncludeBuiltIns: true, Output: artifact,
		Offline: offline, Diagnostics: diagnostics, StroppyRoot: stroppyRoot,
		CacheRoot: store.StroppyRoot(), BuildVersion: buildVersion,
	})
	if err != nil {
		return ActiveRuntime{}, false, err
	}

	finalArtifact := filepath.Join(runtimeDir, "artifacts", manifest.Digest, "stroppy")
	if err := copyArtifactAtomic(artifact, finalArtifact); err != nil {
		return ActiveRuntime{}, false, err
	}

	activeVersion := manifest.StroppyVersion
	if buildVersion != "" {
		activeVersion = buildVersion
	}

	active := ActiveRuntime{
		Schema: runtimeSchemaVersion, BuildDigest: manifest.Digest,
		StroppyVersion: activeVersion, CatalogDigest: catalogDigest,
		ArtifactPath: finalArtifact, BuiltAt: time.Now().UTC(),
	}

	data, err := json.MarshalIndent(active, "", "  ")
	if err != nil {
		return ActiveRuntime{}, false, err
	}

	if err := writeAtomic(filepath.Join(runtimeDir, "active.json"), append(data, '\n'), filePerm); err != nil {
		return ActiveRuntime{}, false, err
	}

	return active, reused, nil
}

// ActiveRuntime returns published local runtime state.
func (store *Store) ActiveRuntime() (ActiveRuntime, error) {
	data, err := os.ReadFile(filepath.Join(store.StroppyRoot(), "runtime", "active.json"))
	if errors.Is(err, os.ErrNotExist) {
		return ActiveRuntime{}, ErrRuntimeNotFound
	}

	if err != nil {
		return ActiveRuntime{}, err
	}

	var active ActiveRuntime
	if err := json.Unmarshal(data, &active); err != nil {
		return ActiveRuntime{}, err
	}

	if active.Schema != runtimeSchemaVersion || active.BuildDigest == "" {
		return ActiveRuntime{}, ErrRuntimeNotFound
	}

	if _, err := os.Stat(active.ArtifactPath); err != nil {
		return ActiveRuntime{}, err
	}

	return active, nil
}

// RuntimeStatus reports active state and whether catalog or Stroppy changed.
func (store *Store) RuntimeStatus(stroppyVersion string) (ActiveRuntime, bool, error) {
	active, err := store.ActiveRuntime()
	if err != nil {
		return ActiveRuntime{}, false, err
	}

	packages, err := store.Packages()
	if err != nil {
		return ActiveRuntime{}, false, err
	}

	digest, err := packageCatalogDigest(packages)
	if err != nil {
		return ActiveRuntime{}, false, err
	}

	return active, active.StroppyVersion != stroppyVersion || active.CatalogDigest != digest, nil
}

// RunRuntime starts current local Stroppy with direct argument and stream forwarding.
func (store *Store) RunRuntime(
	ctx context.Context,
	stroppyVersion string,
	args []string,
	process *RuntimeProcess,
) error {
	active, stale, err := store.RuntimeStatus(stroppyVersion)
	if err != nil {
		return err
	}

	if stale {
		return ErrRuntimeStale
	}

	if process == nil {
		process = &RuntimeProcess{}
	}

	command := exec.CommandContext(ctx, active.ArtifactPath, args...) //nolint:gosec // active path is Stroppy-owned
	command.Stdin = process.Stdin
	command.Stdout = process.Stdout
	command.Stderr = process.Stderr
	command.Env = process.Env

	command.Dir = process.Dir
	if len(command.Env) == 0 {
		command.Env = os.Environ()
	}

	return command.Run()
}

// RemoveRuntime removes active augmented runtime but preserves snapshots and build cache.
func (store *Store) RemoveRuntime() error {
	return os.RemoveAll(filepath.Join(store.StroppyRoot(), "runtime"))
}

func packageCatalogDigest(packages []Package) (string, error) {
	items := make([]struct {
		ImportPath string `json:"import_path"`
		Snapshot   string `json:"snapshot"`
	}, 0, len(packages))
	for _, pkg := range packages {
		items = append(items, struct {
			ImportPath string `json:"import_path"`
			Snapshot   string `json:"snapshot"`
		}{pkg.ImportPath, pkg.SnapshotDigest})
	}

	sort.Slice(items, func(left, right int) bool { return items[left].ImportPath < items[right].ImportPath })

	return canonicalDigest(items)
}
