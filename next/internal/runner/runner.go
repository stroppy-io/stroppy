// Package runner materialises embedded SDK source plus a test's source into a
// throwaway Go module under the user cache and builds it with the go toolchain on
// PATH. It is CLI-only machinery for the stroppy2 command; it is not part of the
// SDK a test imports.
//
// Layout under the cache root (default ~/.cache/stroppy2):
//
//	sdk/<sdkHash>/          materialised SDK (go.mod, go.sum, package dirs); shared
//	build/<buildHash>/      one temp module per (SDK, test source) pair
//	  go.mod go.sum         generated; replace SDK => ../../sdk/<sdkHash>
//	  <test files>          the package's non-test files
//	  testbin               the built executable
//	  .ready                marker written after a successful build
//	gocache/                GOCACHE for the child builds (warm-rebuild cache)
//
// The build directory is content-addressed, so an unchanged (SDK, source) pair
// reuses its binary with no rebuild, and an edit produces a fresh directory whose
// build reuses the shared GOCACHE for the unchanged SDK packages.
package runner

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	next "github.com/stroppy-io/stroppy/next"
)

// Cache is a materialised SDK plus the build directory and toolchain caches. Use
// [NewCache] to construct one; it materialises the SDK on first use.
type Cache struct {
	root     string // cache root (~/.cache/stroppy2)
	sdkDir   string // sdk/<sdkHash>
	sdkHash  string
	sdkGoMod []byte
	goCache  string // gocache
}

// NewCache prepares the cache root and materialises the embedded SDK (once per
// content hash). Repeated calls with an unchanged SDK are cheap: the SDK is
// re-materialised only when its content hash changes.
func NewCache() (*Cache, error) {
	root, err := cacheRoot()
	if err != nil {
		return nil, err
	}
	sdkGoMod, err := next.SDK.ReadFile("go.mod")
	if err != nil {
		return nil, fmt.Errorf("read embedded SDK go.mod: %w", err)
	}
	sdkHash, err := embedHash(next.SDK)
	if err != nil {
		return nil, err
	}
	c := &Cache{
		root:     root,
		sdkDir:   filepath.Join(root, "sdk", sdkHash),
		sdkHash:  sdkHash,
		sdkGoMod: sdkGoMod,
		goCache:  filepath.Join(root, "gocache"),
	}
	if err := c.materializeSDK(); err != nil {
		return nil, err
	}
	return c, nil
}

// cacheRoot returns the stroppy2 cache root, honouring STROPPY2_CACHE for tests.
func cacheRoot() (string, error) {
	if r := os.Getenv("STROPPY2_CACHE"); r != "" {
		return r, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "stroppy2"), nil
}

// materializeSDK writes the embedded SDK tree to sdkDir once, guarded by a .ready
// marker so concurrent or repeated runs do the work at most once per content hash.
func (c *Cache) materializeSDK() error {
	if _, err := os.Stat(filepath.Join(c.sdkDir, ".ready")); err == nil {
		return nil
	}
	if err := writeEmbed(next.SDK, ".", c.sdkDir); err != nil {
		return fmt.Errorf("materialize SDK: %w", err)
	}
	return os.WriteFile(filepath.Join(c.sdkDir, ".ready"), nil, 0o644)
}

// Result reports where a build landed and whether it compiled (vs. reused a
// cached binary).
type Result struct {
	Dir      string // build directory
	Bin      string // built executable
	Compiled bool   // true if go build ran (false if the cached binary was reused)
}

// Build materialises src's package into a content-addressed temp module and
// builds it, returning the executable. A build directory whose .ready marker is
// present is reused as-is (no compile); otherwise the module is written and
// `go build` runs, reusing the shared GOCACHE for unchanged SDK packages.
func (c *Cache) Build(src Source) (Result, error) {
	files := src.buildFiles()
	if len(files) == 0 {
		return Result{}, fmt.Errorf("%s: no buildable source files", src.Name)
	}
	dir := filepath.Join(c.root, "build", buildHash(c.sdkHash, files))
	bin := filepath.Join(dir, "testbin"+exeSuffix())
	if _, err := os.Stat(filepath.Join(dir, ".ready")); err == nil {
		return Result{Dir: dir, Bin: bin, Compiled: false}, nil
	}

	// Fresh (or incomplete) directory: rebuild it from scratch.
	if err := os.RemoveAll(dir); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return Result{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), genGoMod(c.sdkGoMod, c.sdkDir), 0o644); err != nil {
		return Result{}, err
	}
	sum, err := next.SDK.ReadFile("go.sum")
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644); err != nil {
		return Result{}, err
	}
	if err := c.goBuild(dir, bin); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, ".ready"), nil, 0o644); err != nil {
		return Result{}, err
	}
	return Result{Dir: dir, Bin: bin, Compiled: true}, nil
}

// goBuild runs `go build` in dir. -trimpath makes compiled package identity
// path-independent, so the SDK packages hit the shared GOCACHE across different
// build directories (the warm-rebuild win). The module cache is inherited from
// the environment (deps already present); only GOCACHE is redirected so the
// harness's build cache is isolated and observable.
func (c *Cache) goBuild(dir, bin string) error {
	cmd := exec.Command("go", "build", "-trimpath", "-mod=mod", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOCACHE="+c.goCache)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build %s: %w\n%s", dir, err, out)
	}
	return nil
}

// exeSuffix is the platform executable suffix.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// embedHash returns the content hash of every file in an embedded FS.
func embedHash(f fs.FS) (string, error) {
	var paths []string
	err := fs.WalkDir(f, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hashFS(paths, func(p string) ([]byte, error) { return fs.ReadFile(f, p) })
}

// writeEmbed copies the subtree of f rooted at root into dst, skipping _test.go
// files. root "." copies the whole FS.
func writeEmbed(f fs.FS, root, dst string) error {
	return fs.WalkDir(f, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := p
		if root != "." {
			rel = strings.TrimPrefix(p, root+"/")
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := fs.ReadFile(f, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
