package pgnoop

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ulikunitz/xz"
)

func TestAssetName(t *testing.T) {
	for _, tc := range []struct {
		goos, goarch, want string
		wantErr            bool
	}{
		{"linux", "amd64", "pg-noop-x86_64-unknown-linux-musl.tar.xz", false},
		{"linux", "arm64", "pg-noop-aarch64-unknown-linux-musl.tar.xz", false},
		{"darwin", "amd64", "pg-noop-x86_64-apple-darwin.tar.xz", false},
		{"darwin", "arm64", "pg-noop-aarch64-apple-darwin.tar.xz", false},
		{"windows", "amd64", "", true},
	} {
		got, err := AssetName(tc.goos, tc.goarch)
		if tc.wantErr {
			if !errors.Is(err, ErrUnsupportedPlatform) {
				t.Fatalf("AssetName(%s, %s) error = %v, want ErrUnsupportedPlatform", tc.goos, tc.goarch, err)
			}

			continue
		}

		if err != nil || got != tc.want {
			t.Fatalf("AssetName(%s, %s) = %q, %v; want %q", tc.goos, tc.goarch, got, err, tc.want)
		}
	}
}

func TestReleaseURLPinsVersion(t *testing.T) {
	url := ReleaseURL("asset.tar.xz")
	if !strings.Contains(url, "/"+Version+"/") || !strings.HasSuffix(url, "asset.tar.xz") {
		t.Fatalf("ReleaseURL = %q, want pinned %s asset URL", url, Version)
	}
}

func TestVerifySHA256(t *testing.T) {
	data := []byte("payload")
	sum := sha256.Sum256(data)
	sidecar := []byte(hex.EncodeToString(sum[:]) + " *asset.tar.xz\n")

	if err := verifySHA256(data, sidecar, "asset"); err != nil {
		t.Fatalf("verifySHA256() valid sidecar error = %v", err)
	}

	if err := verifySHA256([]byte("tampered"), sidecar, "asset"); err == nil {
		t.Fatal("verifySHA256() accepted a mismatched digest")
	}

	if err := verifySHA256(data, []byte("not-hex\n"), "asset"); err == nil {
		t.Fatal("verifySHA256() accepted a malformed sidecar")
	}

	if err := verifySHA256(data, []byte(""), "asset"); err == nil {
		t.Fatal("verifySHA256() accepted an empty sidecar")
	}
}

// buildTarXz builds a release-shaped tarball: files wrapped in a per-target
// directory with the server binary named pgnoop.
func buildTarXz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var tarBuf bytes.Buffer

	tarWriter := tar.NewWriter(&tarBuf)

	for name, content := range files {
		err := tarWriter.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		})
		if err != nil {
			t.Fatalf("tar header %q: %v", name, err)
		}

		if _, err := tarWriter.Write(content); err != nil {
			t.Fatalf("tar write %q: %v", name, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}

	var xzBuf bytes.Buffer

	xzWriter, err := xz.NewWriter(&xzBuf)
	if err != nil {
		t.Fatalf("xz writer: %v", err)
	}

	if _, err := xzWriter.Write(tarBuf.Bytes()); err != nil {
		t.Fatalf("xz write: %v", err)
	}

	if err := xzWriter.Close(); err != nil {
		t.Fatalf("xz close: %v", err)
	}

	return xzBuf.Bytes()
}

func TestExtractBinary(t *testing.T) {
	binary := []byte("#!/bin/sh\necho pgnoop\n")
	tarball := buildTarXz(t, map[string][]byte{
		"pg-noop-test-target/README.md": []byte("readme"),
		"pg-noop-test-target/pgnoop":    binary,
	})

	got, err := ExtractBinary(tarball)
	if err != nil {
		t.Fatalf("ExtractBinary() error = %v", err)
	}

	if !bytes.Equal(got, binary) {
		t.Fatalf("ExtractBinary() = %q, want the embedded binary", got)
	}

	missing := buildTarXz(t, map[string][]byte{"pg-noop-test-target/README.md": []byte("readme")})
	if _, err := ExtractBinary(missing); !errors.Is(err, errBinaryNotInTarball) {
		t.Fatalf("ExtractBinary() error = %v, want errBinaryNotInTarball", err)
	}

	if _, err := ExtractBinary([]byte("not xz")); err == nil {
		t.Fatal("ExtractBinary() accepted a non-xz payload")
	}
}

func TestResolveExplicitPath(t *testing.T) {
	binary := filepath.Join(t.TempDir(), binaryName)
	if err := os.WriteFile(binary, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(Options{Path: binary, Consent: ConsentNever})
	if err != nil || got != binary {
		t.Fatalf("Resolve() = %q, %v; want explicit path", got, err)
	}

	if _, err := Resolve(Options{Path: filepath.Join(t.TempDir(), "missing"), Consent: ConsentNever}); err == nil {
		t.Fatal("Resolve() accepted a missing explicit path")
	}
}

func TestResolveCacheHitSkipsDownload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cache, err := CachePath()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(cache, []byte("cached"), 0o700); err != nil {
		t.Fatal(err)
	}

	fetchAsset = func(string) ([]byte, error) {
		t.Fatal("download attempted despite cache hit")

		return nil, nil
	}

	t.Cleanup(func() { fetchAsset = fetch })

	got, err := Resolve(Options{Consent: ConsentNever})
	if err != nil || got != cache {
		t.Fatalf("Resolve() = %q, %v; want cache path", got, err)
	}
}

func TestResolveDownloadsAndVerifies(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	binary := []byte("server-binary")
	tarball := buildTarXz(t, map[string][]byte{
		"pg-noop-test-target/pgnoop": binary,
	})
	sum := sha256.Sum256(tarball)

	fetchAsset = func(url string) ([]byte, error) {
		if strings.HasSuffix(url, ".sha256") {
			return []byte(hex.EncodeToString(sum[:]) + " *asset\n"), nil
		}

		return tarball, nil
	}

	t.Cleanup(func() { fetchAsset = fetch })

	got, err := Resolve(Options{Consent: ConsentAlways})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	content, err := os.ReadFile(got)
	if err != nil || !bytes.Equal(content, binary) {
		t.Fatalf("installed binary = %q, %v; want %q", content, err, binary)
	}

	info, err := os.Stat(got)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("installed binary mode = %v, %v; want 0700", info.Mode(), err)
	}
}

func TestResolveRefusesBadDigest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tarball := buildTarXz(t, map[string][]byte{"pg-noop-test-target/pgnoop": []byte("x")})

	fetchAsset = func(url string) ([]byte, error) {
		if strings.HasSuffix(url, ".sha256") {
			return []byte(strings.Repeat("0", 64) + " *asset\n"), nil
		}

		return tarball, nil
	}

	t.Cleanup(func() { fetchAsset = fetch })

	if _, err := Resolve(Options{Consent: ConsentAlways}); err == nil {
		t.Fatal("Resolve() installed a binary with a mismatched digest")
	}
}

func TestResolveRefusesWithoutConsent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	fetchAsset = func(string) ([]byte, error) {
		t.Fatal("download attempted despite ConsentNever")

		return nil, nil
	}

	t.Cleanup(func() { fetchAsset = fetch })

	_, err := Resolve(Options{Consent: ConsentNever})
	if !errors.Is(err, ErrNoServerBinary) {
		t.Fatalf("Resolve() error = %v, want ErrNoServerBinary", err)
	}
}

func TestResolvePromptDeclined(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	declined := false

	_, err := Resolve(Options{
		Consent: ConsentAsk,
		Prompt: func(string) bool {
			declined = true

			return false
		},
	})

	if !declined || !errors.Is(err, ErrNoServerBinary) {
		t.Fatalf("Resolve() declined=%v error=%v, want prompt + ErrNoServerBinary", declined, err)
	}
}

// TestServerLifecycle drives a real pg-noop binary when STROPPY_PG_NOOP_PATH
// points at one; unit environments without the binary skip it.
func TestServerLifecycle(t *testing.T) {
	binary := os.Getenv("STROPPY_PG_NOOP_PATH")
	if binary == "" {
		t.Skip("STROPPY_PG_NOOP_PATH not set")
	}

	port := freeTestPort(t)

	server, err := Start(context.Background(), binary, port)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if server.Port() != port {
		t.Fatalf("Port() = %d, want %d", server.Port(), port)
	}

	conn, err := net.DialTimeout("tcp", server.Addr(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial ready server: %v", err)
	}

	conn.Close()

	if err := server.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if _, err := net.DialTimeout("tcp", server.Addr(), 200*time.Millisecond); err == nil {
		t.Fatal("server still accepting after Stop")
	}
}

func freeTestPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port //nolint:forcetypeassert // loopback addr is TCP
}
