// Package pgnoop manages the external pg-noop blackhole server binary used by
// the baseline command: resolving it from an embedded copy, the local cache,
// or the pinned GitHub release, and running it for the duration of a run.
package pgnoop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Version is the pg-noop release stroppy pins. Bump together with the release
// asset names below when upgrading.
const Version = "v0.1.2"

const (
	releaseBase = "https://github.com/stroppy-io/pg-noop/releases/download/"
	binaryName  = "pgnoop"

	cacheSubDir = "bin"
)

// ErrUnsupportedPlatform is returned when no pg-noop release asset exists for
// the host platform.
var ErrUnsupportedPlatform = errors.New("pgnoop: no release asset for this platform")

// AssetName returns the release asset for a GOOS/GOARCH pair.
func AssetName(goos, goarch string) (string, error) {
	switch {
	case goos == "linux" && goarch == "amd64":
		return "pg-noop-x86_64-unknown-linux-musl.tar.xz", nil
	case goos == "linux" && goarch == "arm64":
		return "pg-noop-aarch64-unknown-linux-musl.tar.xz", nil
	case goos == "darwin" && goarch == "amd64":
		return "pg-noop-x86_64-apple-darwin.tar.xz", nil
	case goos == "darwin" && goarch == "arm64":
		return "pg-noop-aarch64-apple-darwin.tar.xz", nil
	default:
		return "", fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, goos, goarch)
	}
}

// ReleaseURL returns the download URL for a release asset.
func ReleaseURL(asset string) string {
	return releaseBase + Version + "/" + asset
}

// CachePath returns ~/.stroppy/bin/pg-noop/<version>/pgnoop for the host.
func CachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("pgnoop: resolve home dir: %w", err)
	}

	return filepath.Join(home, ".stroppy", cacheSubDir, "pg-noop", Version, binaryName), nil
}

// ExtractBinary reads a pg-noop release tarball (tar.xz) and returns the raw
// server binary. The tarball wraps the binary in a per-target directory.
func ExtractBinary(tarball []byte) ([]byte, error) {
	return extractTarXz(tarball, binaryName)
}

// EmbeddedBinary returns the server binary compiled into stroppy by CI
// release builds (build tag pgnoop_embed), or nil for plain builds.
func EmbeddedBinary() []byte {
	return embeddedBinary
}

// HostTarget describes the host platform for reports and diagnostics.
func HostTarget() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
