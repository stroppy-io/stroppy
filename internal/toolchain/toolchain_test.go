package toolchain

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatibleVersion(t *testing.T) {
	for version, want := range map[string]bool{
		"1.25.9": false,
		"1.26":   false,
		"1.27":   true,
		"1.27.1": true,
		"2.0.0":  true,
		"bad":    false,
	} {
		if got := compatible(version); got != want {
			t.Errorf("compatible(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestPrivateCompilerEnvIsIsolated(t *testing.T) {
	compiler := &Compiler{Root: t.TempDir(), Private: true}
	env := compiler.Env("linux", "arm64", true)

	for key, want := range map[string]string{
		"GOENV": "off", "GOTOOLCHAIN": "local", "GOWORK": "off",
		"GOOS": "linux", "GOARCH": "arm64", "CGO_ENABLED": "0", "GOPROXY": "off",
	} {
		if got := envValue(env, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	for _, key := range []string{"GOPATH", "GOMODCACHE", "GOCACHE", "GOTMPDIR"} {
		if got := envValue(env, key); !strings.HasPrefix(got, compiler.Root) {
			t.Errorf("%s = %q, want under %s", key, got, compiler.Root)
		}
	}
}

func TestSystemCompilerUsesNormalGoCaches(t *testing.T) {
	root := t.TempDir()

	paths := map[string]string{
		"GOPATH": filepath.Join(root, "gopath"), "GOMODCACHE": filepath.Join(root, "modcache"),
		"GOCACHE": filepath.Join(root, "buildcache"), "GOTMPDIR": filepath.Join(root, "tmp"),
	}
	for key, value := range paths {
		if err := os.MkdirAll(value, 0o700); err != nil {
			t.Fatal(err)
		}

		t.Setenv(key, value)
	}

	compiler := &Compiler{Root: root}
	env := compiler.Env("", "", false)

	for key, want := range paths {
		if got := envValue(env, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestCleanPrivateCaches(t *testing.T) {
	root := t.TempDir()

	cache := filepath.Join(root, "go", "modcache", "entry")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}

	toolchain := filepath.Join(root, "toolchains", "go"+PinnedVersion)
	if err := os.MkdirAll(toolchain, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := CleanPrivateCaches(root); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(cache); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private cache remains: %v", err)
	}

	if _, err := os.Stat(toolchain); err != nil {
		t.Fatalf("private toolchain removed: %v", err)
	}
}

func TestCleanPrivateCachesPreservesCachesWithoutPrivateToolchain(t *testing.T) {
	root := t.TempDir()

	cache := filepath.Join(root, "go", "modcache", "entry")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := CleanPrivateCaches(root); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("cache without private toolchain removed: %v", err)
	}
}

func TestResolveUsesCompatibleSystemGo(t *testing.T) {
	compiler, err := Resolve(t.Context(), Options{Root: t.TempDir(), Consent: ConsentNever})
	if err != nil {
		t.Fatal(err)
	}

	if compiler.Private || compiler.Path == "" || !compatible(compiler.Version) {
		t.Fatalf("compiler = %#v", compiler)
	}
}

func TestPromptRequiresTerminal(t *testing.T) {
	allowed, err := prompt(strings.NewReader("yes\n"), &strings.Builder{})
	if allowed || !errors.Is(err, ErrConsentRequired) {
		t.Fatalf("prompt() = %t, %v; want false, ErrConsentRequired", allowed, err)
	}
}

func TestExtractTarGz(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "go.tar.gz")
	writeTarGz(t, archive, []tar.Header{
		{Name: "go/bin/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "go/bin/go", Typeflag: tar.TypeReg, Mode: 0o755, Size: 2},
	}, [][]byte{nil, []byte("go")})

	destination := t.TempDir()
	if err := extractTarGz(archive, destination); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(destination, "go", "bin", "go"))
	if err != nil || string(data) != "go" {
		t.Fatalf("extracted file = %q, %v", data, err)
	}
}

func TestExtractTarGzRejectsUnsafeEntries(t *testing.T) {
	for _, header := range []tar.Header{
		{Name: "../outside", Typeflag: tar.TypeReg},
		{Name: "/outside", Typeflag: tar.TypeReg},
		{Name: "go/link", Typeflag: tar.TypeSymlink, Linkname: "../outside"},
	} {
		archive := filepath.Join(t.TempDir(), "unsafe.tar.gz")
		writeTarGz(t, archive, []tar.Header{header}, [][]byte{nil})

		if err := extractTarGz(archive, t.TempDir()); err == nil {
			t.Fatalf("extractTarGz accepted %#v", header)
		}
	}
}

func writeTarGz(t *testing.T, path string, headers []tar.Header, contents [][]byte) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	compressed := gzip.NewWriter(file)

	archive := tar.NewWriter(compressed)
	for index := range headers {
		if err := archive.WriteHeader(&headers[index]); err != nil {
			t.Fatal(err)
		}

		if _, err := archive.Write(contents[index]); err != nil {
			t.Fatal(err)
		}
	}

	if err := errors.Join(archive.Close(), compressed.Close(), file.Close()); err != nil {
		t.Fatal(err)
	}
}
