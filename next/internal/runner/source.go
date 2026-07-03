package runner

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	next "github.com/stroppy-io/stroppy/next"
)

// Source is a resolved test's source: the package's top-level files keyed by
// base name. A test is a single Go package (package main), so only top-level
// regular files are carried; nested directories and _test.go files are dropped
// (the build ignores tests, and a PoC test is a flat package).
type Source struct {
	// Name identifies the test for cache paths and messages: the built-in name
	// or the target directory's base name.
	Name string
	// Builtin reports whether the source came from the embedded catalog.
	Builtin bool
	// Files maps base name to content, in the flat package directory.
	Files map[string][]byte
}

// BuiltinNames lists the compiled-in test names in catalog order.
func BuiltinNames() []string { return []string{"simple", "tpcc"} }

// IsBuiltin reports whether name is a compiled-in test.
func IsBuiltin(name string) bool {
	for _, n := range BuiltinNames() {
		if n == name {
			return true
		}
	}
	return false
}

// Builtin resolves a compiled-in test's source from the embedded catalog. It
// keeps every embedded file (including _test.go) so the caller can eject a full,
// forkable copy; the build path filters tests separately.
func Builtin(name string) (Source, error) {
	if !IsBuiltin(name) {
		return Source{}, fmt.Errorf("unknown built-in test %q (have %s)", name, strings.Join(BuiltinNames(), ", "))
	}
	dir := path.Join("tests", name)
	files := map[string][]byte{}
	err := fs.WalkDir(next.Builtins, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := next.Builtins.ReadFile(p)
		if err != nil {
			return err
		}
		files[path.Base(p)] = b
		return nil
	})
	if err != nil {
		return Source{}, fmt.Errorf("read built-in %q: %w", name, err)
	}
	return Source{Name: name, Builtin: true, Files: files}, nil
}

// FromPath resolves a user test from a filesystem path. A directory is taken as
// the package directory; a .go file is taken as its containing directory, since
// a Go package lives in a directory and may //go:embed sibling files. Only
// top-level regular files are read.
func FromPath(target string) (Source, error) {
	info, err := os.Stat(target)
	if err != nil {
		return Source{}, err
	}
	dir := target
	if !info.IsDir() {
		if !strings.HasSuffix(target, ".go") {
			return Source{}, fmt.Errorf("%s is not a .go file or directory", target)
		}
		dir = filepath.Dir(target)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Source{}, err
	}
	files := map[string][]byte{}
	var hasGo bool
	for _, e := range entries {
		if e.IsDir() || !e.Type().IsRegular() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".go") {
			hasGo = true
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return Source{}, err
		}
		files[name] = b
	}
	if !hasGo {
		return Source{}, fmt.Errorf("%s contains no .go files", dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return Source{Name: filepath.Base(abs), Files: files}, nil
}

// buildFiles returns the source files that go into a build: everything except
// _test.go, which the compiler ignores and which may pull test-only imports.
func (s Source) buildFiles() map[string][]byte {
	out := make(map[string][]byte, len(s.Files))
	for name, b := range s.Files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		out[name] = b
	}
	return out
}

// sortedNames returns the file names in stable order (for hashing/iteration).
func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
