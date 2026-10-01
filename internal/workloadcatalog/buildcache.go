package workloadcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

const driverBundleVersion = "standard-v1"

type buildIdentity struct {
	Schema          int               `json:"schema"`
	StroppyVersion  string            `json:"stroppy_version"`
	StroppySource   string            `json:"stroppy_source,omitempty"`
	GoVersion       string            `json:"go_version"`
	TargetOS        string            `json:"target_os"`
	TargetArch      string            `json:"target_arch"`
	TargetSettings  map[string]string `json:"target_settings,omitempty"`
	GOFLAGSSHA256   string            `json:"goflags_sha256"`
	IncludeBuiltIns bool              `json:"include_built_ins"`
	Workloads       []string          `json:"workloads"`
	SnapshotDigests []string          `json:"snapshot_digests,omitempty"`
	SourceDigests   []string          `json:"source_digests,omitempty"`
	Modules         []ModuleIdentity  `json:"modules"`
	RunnerSHA256    string            `json:"runner_sha256"`
	DriverBundle    string            `json:"driver_bundle"`
}

// Cache stores completed generated binaries by build identity.
type Cache struct {
	root string
}

// OpenCache resolves Stroppy's content-addressed build cache.
func OpenCache(stroppyRoot string) (*Cache, error) {
	if stroppyRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		stroppyRoot = filepath.Join(home, ".stroppy")
	}

	absolute, err := filepath.Abs(filepath.Join(stroppyRoot, "cache", "builds"))
	if err != nil {
		return nil, err
	}

	return &Cache{root: absolute}, nil
}

// Root returns content-addressed build-cache root.
func (cache *Cache) Root() string {
	if cache == nil {
		return ""
	}

	return cache.root
}

// Clean removes reusable build artifacts while preserving active runtime state.
func (cache *Cache) Clean() error {
	if cache == nil || cache.root == "" {
		return nil
	}

	return os.RemoveAll(cache.root)
}

// Inspect resolves a full build digest or unique prefix.
func (cache *Cache) Inspect(prefix string) (BuildManifest, error) {
	entries, err := os.ReadDir(cache.root)
	if errors.Is(err, fs.ErrNotExist) {
		return BuildManifest{}, ErrBuildNotFound
	}

	if err != nil {
		return BuildManifest{}, err
	}

	matches := make([]string, 0, 1)

	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			matches = append(matches, entry.Name())
		}
	}

	if len(matches) == 0 {
		return BuildManifest{}, ErrBuildNotFound
	}

	if len(matches) > 1 {
		return BuildManifest{}, ErrBuildAmbiguous
	}

	return cache.readManifest(matches[0])
}

func (cache *Cache) lockDigest(digest string) (func() error, error) {
	return (&Store{root: cache.root, lockKey: digest}).lockExclusive()
}

func (cache *Cache) build(
	ctx context.Context,
	compiler *toolchain.Compiler,
	request *RunnerRequest,
) (manifest BuildManifest, artifact string, reused bool, err error) {
	identity, err := runnerIdentity(compiler, request)
	if err != nil {
		return BuildManifest{}, "", false, err
	}

	digest, err := canonicalDigest(identity)
	if err != nil {
		return BuildManifest{}, "", false, err
	}

	if existing, readErr := cache.readManifest(digest); readErr == nil {
		return existing, cache.artifactPath(digest), true, nil
	}

	if err := os.MkdirAll(cache.root, dirPerm); err != nil {
		return BuildManifest{}, "", false, err
	}

	unlock, err := cache.lockDigest(digest)
	if err != nil {
		return BuildManifest{}, "", false, err
	}
	defer func() { err = errors.Join(err, unlock()) }()

	if existing, readErr := cache.readManifest(digest); readErr == nil {
		return existing, cache.artifactPath(digest), true, nil
	}

	staging, err := os.MkdirTemp(cache.root, ".build-*")
	if err != nil {
		return BuildManifest{}, "", false, err
	}
	defer os.RemoveAll(staging)

	artifact = filepath.Join(staging, "stroppy")
	uncached := *request
	uncached.CacheRoot = ""
	uncached.BuildVersion = identity.StroppyVersion
	uncached.BuildDigest = digest

	uncached.Output = artifact
	if err := buildRunnerUncached(ctx, compiler, &uncached); err != nil {
		return BuildManifest{}, "", false, err
	}

	artifactDigest, err := digestFile(artifact)
	if err != nil {
		return BuildManifest{}, "", false, err
	}

	manifest = BuildManifest{
		Schema: BuildSchemaVersion, Digest: digest, CreatedAt: time.Now().UTC(),
		ArtifactSHA256: artifactDigest, StroppyVersion: identity.StroppyVersion,
		StroppySource: identity.StroppySource, GoVersion: identity.GoVersion,
		TargetOS: identity.TargetOS, TargetArch: identity.TargetArch,
		TargetSettings: identity.TargetSettings, GOFLAGSSHA256: identity.GOFLAGSSHA256,
		IncludeBuiltIns: identity.IncludeBuiltIns, Workloads: identity.Workloads,
		SnapshotDigests: identity.SnapshotDigests, SourceDigests: identity.SourceDigests,
		Modules:      identity.Modules,
		RunnerSHA256: identity.RunnerSHA256, DriverBundle: identity.DriverBundle,
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BuildManifest{}, "", false, err
	}

	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), append(data, '\n'), filePerm); err != nil {
		return BuildManifest{}, "", false, err
	}

	entry := filepath.Join(cache.root, digest)
	if err := os.RemoveAll(entry); err != nil {
		return BuildManifest{}, "", false, err
	}

	if err := os.Rename(staging, entry); err != nil {
		if existing, readErr := cache.readManifest(digest); readErr == nil {
			return existing, cache.artifactPath(digest), true, nil
		}

		return BuildManifest{}, "", false, err
	}

	return manifest, cache.artifactPath(digest), false, nil
}

func runnerIdentity(compiler *toolchain.Compiler, request *RunnerRequest) (buildIdentity, error) {
	targetOS, targetArch := effectiveTarget(request.TargetOS, request.TargetArch)
	workloads := make([]string, 0, len(request.Packages))
	snapshots := make([]string, 0, len(request.Packages))
	modules := make([]ModuleIdentity, 0)
	sourceDigests := make([]string, 0, len(request.Packages))

	for _, pkg := range request.Packages {
		workloads = append(workloads, pkg.ImportPath)
		if pkg.SnapshotDigest != "" {
			snapshots = append(snapshots, pkg.SnapshotDigest)
		} else {
			sourceDigest, err := hashPackageInputs(&pkg)
			if err != nil {
				return buildIdentity{}, err
			}

			sourceDigests = append(sourceDigests, sourceDigest)
		}

		requirements, replacements, err := ModuleConfig(&pkg)
		if err != nil {
			return buildIdentity{}, err
		}

		for path, version := range requirements {
			modules = append(modules, ModuleIdentity{Path: path, Version: version})
		}

		for oldModule, newModule := range replacements {
			modules = append(modules, ModuleIdentity{
				Path: oldModule.String(), Replacement: newModule.String(),
			})
		}
	}

	sort.Strings(workloads)
	sort.Strings(snapshots)
	sort.Strings(sourceDigests)
	sort.Slice(modules, func(left, right int) bool {
		if modules[left].Path != modules[right].Path {
			return modules[left].Path < modules[right].Path
		}

		return modules[left].Replacement < modules[right].Replacement
	})

	runnerSource, moduleSource, err := runnerSources(
		request.Packages, request.IncludeBuiltIns, request.StroppyRoot, "", "",
	)
	if err != nil {
		return buildIdentity{}, err
	}

	stroppySource := ""
	if request.StroppyRoot != "" {
		stroppySource, err = hashTree(request.StroppyRoot)
		if err != nil {
			return buildIdentity{}, err
		}
	}

	stroppyVersion := request.BuildVersion
	if stroppyVersion == "" {
		stroppyVersion = stroppyModuleVersion()
	}

	return buildIdentity{
		Schema: BuildSchemaVersion, StroppyVersion: stroppyVersion,
		StroppySource: stroppySource, GoVersion: compiler.Version,
		TargetOS: targetOS, TargetArch: targetArch,
		TargetSettings:  targetSettings(targetOS, targetArch),
		GOFLAGSSHA256:   digestBytes([]byte(os.Getenv("GOFLAGS"))),
		IncludeBuiltIns: request.IncludeBuiltIns, Workloads: workloads,
		SnapshotDigests: snapshots, SourceDigests: sourceDigests, Modules: modules,
		RunnerSHA256: digestBytes(append(runnerSource, moduleSource...)),
		DriverBundle: driverBundleVersion,
	}, nil
}

func hashPackageInputs(pkg *Package) (string, error) {
	modules, err := discoverLocalModules(pkg.ModuleRoot)
	if err != nil {
		return "", err
	}

	type moduleDigest struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}

	values := make([]moduleDigest, 0, len(modules))
	for _, module := range modules {
		digest, err := hashTree(module.root)
		if err != nil {
			return "", err
		}

		values = append(values, moduleDigest{Path: module.path, SHA256: digest})
	}

	sort.Slice(values, func(left, right int) bool { return values[left].Path < values[right].Path })

	return canonicalDigest(values)
}

func hashTree(root string) (string, error) {
	type fileDigest struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}

	files := make([]fileDigest, 0)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".hg" || entry.Name() == ".svn" ||
				entry.Name() == ".claude" || relative == "build" {
				return filepath.SkipDir
			}

			return nil
		}

		if relative == "coverage.out" || relative == "cpu.out" || relative == "mem.out" {
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}

		digest, err := digestFile(path)
		if err != nil {
			return err
		}

		files = append(files, fileDigest{Path: filepath.ToSlash(relative), SHA256: digest})

		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })

	return canonicalDigest(files)
}

func (cache *Cache) readManifest(digest string) (BuildManifest, error) {
	data, err := os.ReadFile(filepath.Join(cache.root, digest, "manifest.json"))
	if err != nil {
		return BuildManifest{}, err
	}

	var manifest BuildManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return BuildManifest{}, err
	}

	if manifest.Schema != BuildSchemaVersion || manifest.Digest != digest {
		return BuildManifest{}, ErrBuildNotFound
	}

	artifact := cache.artifactPath(digest)

	artifactDigest, err := digestFile(artifact)
	if err != nil || artifactDigest != manifest.ArtifactSHA256 {
		return BuildManifest{}, ErrBuildNotFound
	}

	return manifest, nil
}

func (cache *Cache) artifactPath(digest string) string {
	return filepath.Join(cache.root, digest, "stroppy")
}

// BuildCached builds or reuses one runner and materializes it atomically at request.Output.
func BuildCached(
	ctx context.Context,
	compiler *toolchain.Compiler,
	request *RunnerRequest,
) (BuildManifest, bool, error) {
	cache, err := OpenCache(request.CacheRoot)
	if err != nil {
		return BuildManifest{}, false, err
	}

	manifest, artifact, reused, err := cache.build(ctx, compiler, request)
	if err != nil {
		return BuildManifest{}, false, err
	}

	if err := copyArtifactAtomic(artifact, request.Output); err != nil {
		return BuildManifest{}, false, err
	}

	return manifest, reused, nil
}

func copyArtifactAtomic(source, destination string) (returnErr error) {
	if err := os.MkdirAll(filepath.Dir(destination), runnerDirPerm); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".stroppy-build-*")
	if err != nil {
		return err
	}

	path := temporary.Name()

	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, temporary.Close(), os.Remove(path))
		}
	}()

	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	if _, err := io.Copy(temporary, input); err != nil {
		return err
	}

	if err := temporary.Chmod(runnerDirPerm); err != nil {
		return err
	}

	if err := temporary.Sync(); err != nil {
		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(path, destination)
}

// InspectBuild resolves one build manifest in Stroppy's cache.
func InspectBuild(stroppyRoot, prefix string) (BuildManifest, error) {
	cache, err := OpenCache(stroppyRoot)
	if err != nil {
		return BuildManifest{}, err
	}

	return cache.Inspect(prefix)
}

// CleanBuildCache removes reusable content-addressed artifacts.
func CleanBuildCache(stroppyRoot string) error {
	cache, err := OpenCache(stroppyRoot)
	if err != nil {
		return err
	}

	return cache.Clean()
}
