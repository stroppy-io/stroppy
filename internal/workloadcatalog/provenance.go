package workloadcatalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/modfile"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

const (
	BuildSchemaVersion    = 1
	SnapshotSchemaVersion = 1
)

var (
	ErrBuildNotFound    = errors.New("build cache entry not found")
	ErrBuildAmbiguous   = errors.New("build digest prefix is ambiguous")
	errUnsafeSource     = errors.New("source input resolves outside module root")
	errLocalReplacement = errors.New("local replacement module is unavailable")
)

// ModuleIdentity is one resolved module dependency or replacement.
type ModuleIdentity struct {
	Path        string `json:"path"`
	Version     string `json:"version,omitempty"`
	Sum         string `json:"sum,omitempty"`
	Replacement string `json:"replacement,omitempty"`
}

// SnapshotFile identifies one immutable compiler input.
type SnapshotFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// SnapshotModule describes one local module copied into a snapshot.
type SnapshotModule struct {
	Path      string         `json:"path"`
	Directory string         `json:"directory"`
	Files     []SnapshotFile `json:"files"`
}

// Snapshot describes immutable workload compiler inputs owned by Stroppy.
type Snapshot struct {
	Schema           int              `json:"schema"`
	Digest           string           `json:"digest"`
	ImportPath       string           `json:"import_path"`
	ModulePath       string           `json:"module_path"`
	PackageDirectory string           `json:"package_directory"`
	Modules          []SnapshotModule `json:"modules"`
}

// BuildManifest records non-secret inputs and output integrity for one cached executable.
type BuildManifest struct {
	Schema          int               `json:"schema"`
	Digest          string            `json:"digest"`
	CreatedAt       time.Time         `json:"created_at"`
	ArtifactSHA256  string            `json:"artifact_sha256"`
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

//nolint:tagliatelle // Go command JSON uses exported Go field names.
type listedPackage struct {
	Dir               string        `json:"Dir"`
	GoFiles           []string      `json:"GoFiles"`
	IgnoredGoFiles    []string      `json:"IgnoredGoFiles"`
	IgnoredOtherFiles []string      `json:"IgnoredOtherFiles"`
	CgoFiles          []string      `json:"CgoFiles"`
	CFiles            []string      `json:"CFiles"`
	CXXFiles          []string      `json:"CXXFiles"`
	MFiles            []string      `json:"MFiles"`
	HFiles            []string      `json:"HFiles"`
	FFiles            []string      `json:"FFiles"`
	SFiles            []string      `json:"SFiles"`
	SwigFiles         []string      `json:"SwigFiles"`
	SwigCXXFiles      []string      `json:"SwigCXXFiles"`
	SysoFiles         []string      `json:"SysoFiles"`
	EmbedFiles        []string      `json:"EmbedFiles"`
	Module            *listedModule `json:"Module"`
}

//nolint:tagliatelle // Go command JSON uses exported Go field names.
type listedModule struct {
	Path    string        `json:"Path"`
	Dir     string        `json:"Dir"`
	Version string        `json:"Version"`
	Sum     string        `json:"Sum"`
	Replace *listedModule `json:"Replace"`
}

type localModule struct {
	path  string
	root  string
	files map[string][]byte
}

func digestBytes(data []byte) string {
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

func digestFile(path string) (string, error) {
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

func canonicalDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	return digestBytes(data), nil
}

func targetSettings(targetOS, targetArch string) map[string]string {
	settings := map[string]string{}

	for _, key := range []string{
		"GO386", "GOAMD64", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64",
		"GOPPC64", "GORISCV64", "GOWASM", "GOEXPERIMENT", "GOFIPS140",
	} {
		if value := os.Getenv(key); value != "" {
			settings[key] = value
		}
	}

	settings["GOOS"] = targetOS
	settings["GOARCH"] = targetArch
	settings["CGO_ENABLED"] = "0"

	return settings
}

func effectiveTarget(targetOS, targetArch string) (effectiveOS, effectiveArch string) {
	effectiveOS = targetOS
	if effectiveOS == "" {
		effectiveOS = os.Getenv("GOOS")
	}

	if effectiveOS == "" {
		effectiveOS = runtime.GOOS
	}

	effectiveArch = targetArch
	if effectiveArch == "" {
		effectiveArch = os.Getenv("GOARCH")
	}

	if effectiveArch == "" {
		effectiveArch = runtime.GOARCH
	}

	return effectiveOS, effectiveArch
}

// CreateSnapshot copies exact local compiler inputs into Stroppy-owned storage.
func (store *Store) CreateSnapshot(
	ctx context.Context,
	compiler *toolchain.Compiler,
	pkg *Package,
	offline bool,
) (Snapshot, Package, error) {
	modules, packageFiles, err := discoverLocalInputs(ctx, compiler, pkg, offline)
	if err != nil {
		return Snapshot{}, Package{}, err
	}

	moduleIDs := make(map[string]string, len(modules))

	roots := make(map[string]string, len(modules))
	for _, local := range modules {
		moduleIDs[local.path] = digestBytes([]byte(local.path))[:16]
		roots[local.root] = local.path
	}

	for _, local := range modules {
		if err := loadModuleFiles(local, packageFiles[local.root]); err != nil {
			return Snapshot{}, Package{}, err
		}

		if err := rewriteLocalReplacements(local, moduleIDs, roots); err != nil {
			return Snapshot{}, Package{}, err
		}
	}

	packageRelative, err := filepath.Rel(pkg.ModuleRoot, pkg.Directory)
	if err != nil {
		return Snapshot{}, Package{}, err
	}

	snapshot := Snapshot{
		Schema: SnapshotSchemaVersion, ImportPath: pkg.ImportPath, ModulePath: pkg.ModulePath,
		PackageDirectory: filepath.ToSlash(packageRelative),
		Modules:          make([]SnapshotModule, 0, len(modules)),
	}
	for _, local := range modules {
		moduleSnapshot := SnapshotModule{
			Path: local.path, Directory: moduleIDs[local.path],
			Files: make([]SnapshotFile, 0, len(local.files)),
		}
		for path, data := range local.files {
			moduleSnapshot.Files = append(moduleSnapshot.Files, SnapshotFile{
				Path: filepath.ToSlash(path), SHA256: digestBytes(data),
			})
		}

		sort.Slice(moduleSnapshot.Files, func(left, right int) bool {
			return moduleSnapshot.Files[left].Path < moduleSnapshot.Files[right].Path
		})
		snapshot.Modules = append(snapshot.Modules, moduleSnapshot)
	}

	sort.Slice(snapshot.Modules, func(left, right int) bool {
		return snapshot.Modules[left].Path < snapshot.Modules[right].Path
	})

	digest, err := canonicalDigest(snapshot)
	if err != nil {
		return Snapshot{}, Package{}, err
	}

	snapshot.Digest = digest

	if err := store.publishSnapshot(&snapshot, modules); err != nil {
		return Snapshot{}, Package{}, err
	}

	snapshotPackage, err := store.snapshotPackage(&snapshot)
	if err != nil {
		return Snapshot{}, Package{}, err
	}

	return snapshot, snapshotPackage, nil
}

func discoverLocalInputs(
	ctx context.Context,
	compiler *toolchain.Compiler,
	pkg *Package,
	offline bool,
) ([]*localModule, map[string]map[string]struct{}, error) {
	modules, err := discoverLocalModules(pkg.ModuleRoot)
	if err != nil {
		return nil, nil, err
	}

	command := exec.CommandContext( //nolint:gosec // compiler path comes from verified resolver
		ctx, compiler.Path, "list", "-mod=readonly", "-deps", "-json", ".",
	)
	command.Dir = pkg.Directory
	command.Env = compiler.Env("", "", offline)

	var stdout, stderr bytes.Buffer

	command.Stdout = &stdout

	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, nil, fmt.Errorf("inspect workload inputs: %w: %s", err, stderr.String())
	}

	roots := make(map[string]string, len(modules))
	for _, local := range modules {
		roots[local.root] = local.path
	}

	files := make(map[string]map[string]struct{}, len(modules))
	decoder := json.NewDecoder(&stdout)

	for {
		var listed listedPackage
		if err := decoder.Decode(&listed); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, nil, err
		}

		root := effectiveModuleRoot(listed.Module)
		if _, local := roots[root]; !local {
			continue
		}

		if files[root] == nil {
			files[root] = map[string]struct{}{}
		}

		for _, name := range listed.files() {
			path := name
			if !filepath.IsAbs(path) {
				path = filepath.Join(listed.Dir, path)
			}

			relative, err := filepath.Rel(root, path)
			if err != nil || filepath.IsAbs(relative) || relative == ".." ||
				strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
				return nil, nil, fmt.Errorf("%w: %s", errUnsafeSource, path)
			}

			files[root][filepath.Clean(relative)] = struct{}{}
		}
	}

	return modules, files, nil
}

func (listed *listedPackage) files() []string {
	count := len(listed.GoFiles) + len(listed.IgnoredGoFiles) + len(listed.IgnoredOtherFiles) +
		len(listed.CgoFiles) + len(listed.CFiles) + len(listed.CXXFiles) +
		len(listed.MFiles) + len(listed.HFiles) + len(listed.FFiles) +
		len(listed.SFiles) + len(listed.SwigFiles) + len(listed.SwigCXXFiles) +
		len(listed.SysoFiles) + len(listed.EmbedFiles)

	files := make([]string, 0, count)
	for _, group := range [][]string{
		listed.GoFiles, listed.IgnoredGoFiles, listed.IgnoredOtherFiles,
		listed.CgoFiles, listed.CFiles, listed.CXXFiles, listed.MFiles,
		listed.HFiles, listed.FFiles, listed.SFiles, listed.SwigFiles,
		listed.SwigCXXFiles, listed.SysoFiles, listed.EmbedFiles,
	} {
		files = append(files, group...)
	}

	return files
}

func effectiveModuleRoot(module *listedModule) string {
	if module == nil {
		return ""
	}

	if module.Replace != nil && module.Replace.Dir != "" {
		return cleanExistingPath(module.Replace.Dir)
	}

	return cleanExistingPath(module.Dir)
}

func cleanExistingPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}

	absolute, _ := filepath.Abs(path)

	return absolute
}

//nolint:gosec // all discovered roots come from parsed local replace directives
func discoverLocalModules(root string) ([]*localModule, error) {
	pending := []string{cleanExistingPath(root)}
	byRoot := map[string]*localModule{}

	for len(pending) > 0 {
		moduleRoot := pending[0]
		pending = pending[1:]

		if _, exists := byRoot[moduleRoot]; exists {
			continue
		}

		path := filepath.Join(moduleRoot, "go.mod")

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		file, err := modfile.Parse(path, data, nil)
		if err != nil {
			return nil, err
		}

		if file.Module == nil || file.Module.Mod.Path == "" {
			return nil, ErrMissingModulePath
		}

		local := &localModule{path: file.Module.Mod.Path, root: moduleRoot, files: map[string][]byte{}}
		byRoot[moduleRoot] = local

		for _, replacement := range file.Replace {
			if replacement.Old.Path == stroppyModulePath || !modfile.IsDirectoryPath(replacement.New.Path) {
				continue
			}

			replacementRoot := replacement.New.Path
			if !filepath.IsAbs(replacementRoot) {
				replacementRoot = filepath.Join(moduleRoot, replacementRoot)
			}

			pending = append(pending, cleanExistingPath(replacementRoot))
		}
	}

	modules := make([]*localModule, 0, len(byRoot))
	for _, local := range byRoot {
		modules = append(modules, local)
	}

	sort.Slice(modules, func(left, right int) bool { return modules[left].path < modules[right].path })

	return modules, nil
}

func loadModuleFiles(local *localModule, files map[string]struct{}) error {
	if files == nil {
		files = map[string]struct{}{}
	}

	files["go.mod"] = struct{}{}
	if _, err := os.Stat(filepath.Join(local.root, "go.sum")); err == nil {
		files["go.sum"] = struct{}{}
	}

	moduleRoot, err := filepath.EvalSymlinks(local.root)
	if err != nil {
		return err
	}

	for relative := range files {
		path := filepath.Join(moduleRoot, relative)

		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}

		inside, err := filepath.Rel(moduleRoot, resolved)
		if err != nil || filepath.IsAbs(inside) || inside == ".." ||
			strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("%w: %s", errUnsafeSource, path)
		}

		data, err := os.ReadFile(resolved)
		if err != nil {
			return err
		}

		local.files[filepath.Clean(relative)] = data
	}

	return nil
}

func rewriteLocalReplacements(
	local *localModule,
	moduleIDs, roots map[string]string,
) error {
	path := filepath.Join(local.root, "go.mod")

	file, err := modfile.Parse(path, local.files["go.mod"], nil)
	if err != nil {
		return err
	}

	for _, replacement := range file.Replace {
		if replacement.Old.Path == stroppyModulePath {
			if err := file.DropReplace(replacement.Old.Path, replacement.Old.Version); err != nil {
				return err
			}

			continue
		}

		if !modfile.IsDirectoryPath(replacement.New.Path) {
			continue
		}

		replacementRoot := replacement.New.Path
		if !filepath.IsAbs(replacementRoot) {
			replacementRoot = filepath.Join(local.root, replacementRoot)
		}

		targetPath, ok := roots[cleanExistingPath(replacementRoot)]
		if !ok {
			return fmt.Errorf("%w: %s", errLocalReplacement, replacement.New.Path)
		}

		from := filepath.Join("modules", moduleIDs[local.path])
		to := filepath.Join("modules", moduleIDs[targetPath])

		relative, err := filepath.Rel(from, to)
		if err != nil {
			return err
		}

		if err := file.AddReplace(
			replacement.Old.Path, replacement.Old.Version, filepath.ToSlash(relative), "",
		); err != nil {
			return err
		}
	}

	formatted, err := file.Format()
	if err != nil {
		return err
	}

	local.files["go.mod"] = formatted

	return nil
}

func (store *Store) publishSnapshot(snapshot *Snapshot, modules []*localModule) (returnErr error) {
	root := filepath.Join(store.snapshotsDir(), snapshot.Digest)
	if existing, err := store.readSnapshot(snapshot.Digest); err == nil && existing.Digest == snapshot.Digest {
		return nil
	}

	if err := os.MkdirAll(store.snapshotsDir(), dirPerm); err != nil {
		return err
	}

	staging, err := os.MkdirTemp(store.snapshotsDir(), ".snapshot-*")
	if err != nil {
		return err
	}

	defer func() { returnErr = errors.Join(returnErr, os.RemoveAll(staging)) }()

	moduleByPath := make(map[string]*localModule, len(modules))
	for _, local := range modules {
		moduleByPath[local.path] = local
	}

	for _, moduleSnapshot := range snapshot.Modules {
		local := moduleByPath[moduleSnapshot.Path]
		for _, file := range moduleSnapshot.Files {
			data := local.files[filepath.FromSlash(file.Path)]

			target := filepath.Join(staging, "modules", moduleSnapshot.Directory, filepath.FromSlash(file.Path))
			if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
				return err
			}

			if err := os.WriteFile(target, data, filePerm); err != nil {
				return err
			}
		}
	}

	manifest, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), append(manifest, '\n'), filePerm); err != nil {
		return err
	}

	if err := os.Rename(staging, root); err != nil {
		if _, statErr := os.Stat(root); statErr == nil {
			return nil
		}

		return err
	}

	return nil
}

func (store *Store) readSnapshot(digest string) (Snapshot, error) {
	data, err := os.ReadFile(filepath.Join(store.snapshotsDir(), digest, "manifest.json"))
	if err != nil {
		return Snapshot{}, err
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}

	if snapshot.Schema != SnapshotSchemaVersion || snapshot.Digest != digest {
		return Snapshot{}, ErrInvalidEntry
	}

	return snapshot, nil
}

// Package resolves one owned source snapshot to an importable package.
func (store *Store) Package(digest string) (Package, error) {
	snapshot, err := store.readSnapshot(digest)
	if err != nil {
		return Package{}, err
	}

	return store.snapshotPackage(&snapshot)
}

func (store *Store) snapshotPackage(snapshot *Snapshot) (Package, error) {
	var module SnapshotModule

	for _, candidate := range snapshot.Modules {
		if candidate.Path == snapshot.ModulePath {
			module = candidate

			break
		}
	}

	if module.Path == "" {
		return Package{}, ErrMissingModulePath
	}

	root := filepath.Join(store.snapshotsDir(), snapshot.Digest, "modules", module.Directory)

	return Package{
		ImportPath:     snapshot.ImportPath,
		Directory:      filepath.Join(root, filepath.FromSlash(snapshot.PackageDirectory)),
		ModulePath:     snapshot.ModulePath,
		ModuleRoot:     root,
		SnapshotDigest: snapshot.Digest,
	}, nil
}
