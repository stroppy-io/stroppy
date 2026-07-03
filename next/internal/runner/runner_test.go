package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	next "github.com/stroppy-io/stroppy/next"
)

func TestGenGoModGolden(t *testing.T) {
	sdkGoMod := []byte(`module github.com/stroppy-io/stroppy/next

go 1.26

require github.com/jackc/pgx/v5 v5.10.0

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
`)
	want := `module usertest

go 1.26

require github.com/jackc/pgx/v5 v5.10.0

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

require github.com/stroppy-io/stroppy/next v0.0.0-00010101000000-000000000000

replace github.com/stroppy-io/stroppy/next => /SDK
`
	if got := string(genGoMod(sdkGoMod, "/SDK")); got != want {
		t.Errorf("genGoMod mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestGenGoModUsesRealSDK(t *testing.T) {
	// The real embedded SDK go.mod must produce a module usertest with a replace
	// to the given dir, and must still name pgx (the sole direct dep).
	got := string(genGoMod(mustReadSDK(t, "go.mod"), "/x/y"))
	for _, want := range []string{
		"module usertest\n",
		"replace github.com/stroppy-io/stroppy/next => /x/y\n",
		"github.com/jackc/pgx/v5",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated go.mod missing %q:\n%s", want, got)
		}
	}
}

func TestBuildHashStable(t *testing.T) {
	files := map[string][]byte{"main.go": []byte("package main"), "q.sql": []byte("--+ a")}
	h1 := buildHash("sdkA", files)
	// Same inputs (new map, same content) → same hash: order-independent.
	h2 := buildHash("sdkA", map[string][]byte{"q.sql": []byte("--+ a"), "main.go": []byte("package main")})
	if h1 != h2 {
		t.Errorf("hash not stable: %s != %s", h1, h2)
	}
	// Any change flips the hash.
	if buildHash("sdkB", files) == h1 {
		t.Error("hash ignored SDK identity")
	}
	if buildHash("sdkA", map[string][]byte{"main.go": []byte("package main // edit")}) == h1 {
		t.Error("hash ignored source content")
	}
	if buildHash("sdkA", map[string][]byte{"main.go": []byte("package main"), "extra.go": nil}) == h1 {
		t.Error("hash ignored file set")
	}
}

func TestBuildHashDistinguishesFileBoundary(t *testing.T) {
	// "ab"+"c" must differ from "a"+"bc": length framing prevents concatenation
	// collisions across the file set.
	a := buildHash("s", map[string][]byte{"x": []byte("ab"), "y": []byte("c")})
	b := buildHash("s", map[string][]byte{"x": []byte("a"), "y": []byte("bc")})
	if a == b {
		t.Error("hash collides across a shifted file boundary")
	}
}

func TestBuiltinResolves(t *testing.T) {
	for _, name := range BuiltinNames() {
		src, err := Builtin(name)
		if err != nil {
			t.Fatalf("Builtin(%q): %v", name, err)
		}
		if !src.Builtin || src.Name != name {
			t.Errorf("Builtin(%q) = %+v", name, src)
		}
		if _, ok := src.Files["main.go"]; !ok {
			t.Errorf("Builtin(%q) has no main.go", name)
		}
	}
	if _, err := Builtin("nope"); err == nil {
		t.Error("Builtin(nope) should error")
	}
}

func TestBuildFilesDropsTests(t *testing.T) {
	src := Source{Files: map[string][]byte{
		"main.go":       []byte("x"),
		"main_test.go":  []byte("x"),
		"gen.go":        []byte("x"),
		"alloc_test.go": []byte("x"),
	}}
	bf := src.buildFiles()
	if _, ok := bf["main_test.go"]; ok {
		t.Error("buildFiles kept a _test.go file")
	}
	if len(bf) != 2 {
		t.Errorf("buildFiles = %v, want main.go+gen.go", bf)
	}
}

func TestFromPath(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "main.go"), "package main\nfunc main(){}\n")
	write(t, filepath.Join(dir, "q.sql"), "--+ s\n")
	write(t, filepath.Join(dir, "x_test.go"), "package main\n")

	// Directory target.
	src, err := FromPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if src.Builtin {
		t.Error("path source marked builtin")
	}
	for _, want := range []string{"main.go", "q.sql", "x_test.go"} {
		if _, ok := src.Files[want]; !ok {
			t.Errorf("FromPath missing %q", want)
		}
	}
	// _test.go is carried in Files but dropped from the build set.
	if _, ok := src.buildFiles()["x_test.go"]; ok {
		t.Error("buildFiles kept x_test.go")
	}

	// .go file target resolves to its directory.
	src2, err := FromPath(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := src2.Files["q.sql"]; !ok {
		t.Error(".go target did not pick up sibling q.sql")
	}
}

func TestFromPathRejectsNoGo(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "readme.txt"), "hi")
	if _, err := FromPath(dir); err == nil {
		t.Error("FromPath should reject a directory with no .go files")
	}
	if _, err := FromPath(filepath.Join(dir, "readme.txt")); err == nil {
		t.Error("FromPath should reject a non-.go file")
	}
}

func TestEjectRefusesNonEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")

	// Empty/missing target succeeds and drops a README plus the source.
	files, err := Eject("simple", dir)
	if err != nil {
		t.Fatalf("Eject into fresh dir: %v", err)
	}
	if !contains(files, "README.md") || !contains(files, "main.go") {
		t.Errorf("Eject wrote %v, want README.md and main.go", files)
	}

	// Second eject into the now-populated dir is refused.
	if _, err := Eject("simple", dir); err == nil {
		t.Error("Eject into non-empty dir should be refused")
	}
}

func TestEjectUnknownBuiltin(t *testing.T) {
	if _, err := Eject("nope", filepath.Join(t.TempDir(), "x")); err == nil {
		t.Error("Eject of unknown builtin should error")
	}
}

func mustReadSDK(t *testing.T, name string) []byte {
	t.Helper()
	b, err := next.SDK.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
