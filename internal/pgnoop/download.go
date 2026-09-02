package pgnoop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	downloadTimeout = 5 * time.Minute
	binaryPerm      = 0o700
	dirPerm         = 0o755
)

var (
	errDigestMalformed = errors.New("pgnoop: malformed pinned sha256 digest")
	errDigestMismatch  = errors.New("pgnoop: sha256 digest mismatch")
)

// fetchAsset downloads one URL; tests swap it for a local source.
var fetchAsset = fetch

// assetDigest resolves the expected digest for an asset; tests swap it.
var assetDigest = AssetDigest

// download fetches the pinned release asset and verifies it against the
// digest compiled into stroppy's own source, then installs the binary at
// cachePath. The digest is intentionally not fetched from the release
// source, so a tampered release cannot pass verification.
func download(cachePath string, opts Options) (string, error) {
	asset, err := AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}

	pinnedDigest, err := assetDigest(asset)
	if err != nil {
		return "", err
	}

	if opts.Consent == ConsentNever {
		return "", fmt.Errorf(
			"%w: downloads disabled; place the binary at %s or pass --server-path",
			ErrNoServerBinary, cachePath,
		)
	}

	if opts.Consent == ConsentAsk &&
		!prompt(opts, fmt.Sprintf(
			"Download pg-noop %s (%s) from github.com/stroppy-io/pg-noop to %s?",
			Version, asset, filepath.Dir(cachePath),
		)) {
		return "", fmt.Errorf(
			"%w: download declined; place the binary at %s or pass --server-path",
			ErrNoServerBinary, cachePath,
		)
	}

	logf(opts, "downloading %s", ReleaseURL(asset))

	tarball, err := fetchAsset(ReleaseURL(asset))
	if err != nil {
		return "", err
	}

	if err := verifySHA256(tarball, pinnedDigest, asset); err != nil {
		return "", err
	}

	binary, err := ExtractBinary(tarball)
	if err != nil {
		return "", err
	}

	if err := writeBinary(cachePath, binary); err != nil {
		return "", err
	}

	logf(opts, "installed %s", cachePath)

	return cachePath, nil
}

func fetch(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("pgnoop: build request for %s: %w", url, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pgnoop: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pgnoop: fetch %s: %s", url, resp.Status) //nolint:err113 // status text is dynamic
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pgnoop: read %s: %w", url, err)
	}

	return data, nil
}

// verifySHA256 checks data against a pinned hex digest. Wrong-length digests
// are rejected before the fixed-size comparison so a short digest cannot
// panic the conversion.
func verifySHA256(data []byte, wantHex, name string) error {
	want, err := hex.DecodeString(wantHex)
	if err != nil || len(want) != sha256.Size {
		return fmt.Errorf("%w: %s", errDigestMalformed, name)
	}

	got := sha256.Sum256(data)
	if !bytes.Equal(got[:], want) {
		return fmt.Errorf("%w: %s", errDigestMismatch, name)
	}

	return nil
}

// writeBinary writes data to path atomically (tmp + rename) with execute permission.
func writeBinary(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("pgnoop: create cache dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".pgnoop-*")
	if err != nil {
		return fmt.Errorf("pgnoop: create temp file: %w", err)
	}

	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)

		return fmt.Errorf("pgnoop: write binary: %w", err)
	}

	if err := tmp.Chmod(binaryPerm); err != nil {
		tmp.Close()
		os.Remove(tmpName)

		return fmt.Errorf("pgnoop: chmod binary: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)

		return fmt.Errorf("pgnoop: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)

		return fmt.Errorf("pgnoop: install binary: %w", err)
	}

	return nil
}

// askTerminal asks the user on an interactive terminal; non-interactive
// callers get a refusal.
func askTerminal(question string) bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil || stdinInfo.Mode()&os.ModeCharDevice == 0 {
		fmt.Fprintf(os.Stderr, "%s [non-interactive terminal: skipped]\n", question)

		return false
	}

	fmt.Fprintf(os.Stderr, "%s [y/N] ", question)

	var answer string
	if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil {
		return false
	}

	answer = strings.ToLower(strings.TrimSpace(answer))

	return answer == "y" || answer == "yes"
}
