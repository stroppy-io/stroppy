package toolchain

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const archiveFilePerm = 0o755

var (
	downloadClient = http.DefaultClient

	errDownloadStatus    = errors.New("go toolchain download failed")
	errChecksumMismatch  = errors.New("go toolchain checksum mismatch")
	errUnsafeArchivePath = errors.New("unsafe Go toolchain archive path")
	errArchiveEntry      = errors.New("unsupported Go toolchain archive entry")
)

func install(ctx context.Context, root string, output io.Writer) (returnErr error) {
	release, err := hostRelease()
	if err != nil {
		return err
	}

	toolchains := filepath.Join(root, "toolchains")
	if err := os.MkdirAll(toolchains, privateDirPerm); err != nil {
		return err
	}

	unlock, err := lockInstall(toolchains)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, unlock()) }()

	compiler := privateCompiler(root)
	if privateAvailable(ctx, compiler) {
		return nil
	}

	archivePath := filepath.Join(toolchains, release.File)
	if validDigest(archivePath, release.SHA256) {
		if output != nil {
			fmt.Fprintf(output, "using cached %s\n", archivePath)
		}
	} else {
		if output != nil {
			fmt.Fprintf(output, "downloading %s%s\n", downloadBaseURL, release.File)
		}

		if err := download(ctx, downloadBaseURL+release.File, archivePath, release.SHA256); err != nil {
			return err
		}
	}

	temporary, err := os.MkdirTemp(toolchains, ".go-install-*")
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, os.RemoveAll(temporary)) }()

	if err := extractTarGz(archivePath, temporary); err != nil {
		return err
	}

	destination := filepath.Join(toolchains, "go"+PinnedVersion)
	if err := os.RemoveAll(destination); err != nil {
		return err
	}

	return os.Rename(temporary, destination)
}

func download(ctx context.Context, sourceURL, destination, checksum string) (returnErr error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, http.NoBody)
	if err != nil {
		return err
	}

	response, err := downloadClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s", errDownloadStatus, response.Status)
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".go-download-*")
	if err != nil {
		return err
	}

	path := temporary.Name()

	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, temporary.Close(), os.Remove(path))
		}
	}()

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temporary, hash), response.Body); err != nil {
		return err
	}

	if hex.EncodeToString(hash.Sum(nil)) != checksum {
		return errChecksumMismatch
	}

	if err := temporary.Sync(); err != nil {
		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(path, destination)
}

func validDigest(path, expected string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}

	return hex.EncodeToString(hash.Sum(nil)) == expected
}

//nolint:cyclop,gocognit // archive validation and extraction stay together
func extractTarGz(path, destination string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	compressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer compressed.Close()

	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}

		name := filepath.Clean(filepath.FromSlash(header.Name))
		if name == "." || filepath.IsAbs(name) || filepath.VolumeName(name) != "" || name == ".." ||
			strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("%w %q", errUnsafeArchivePath, header.Name)
		}

		target := filepath.Join(destination, name)

		relative, err := filepath.Rel(destination, target)
		if err != nil || filepath.IsAbs(relative) || relative == ".." ||
			strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("%w %q", errUnsafeArchivePath, header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, privateDirPerm); err != nil {
				return err
			}
		case tar.TypeReg:
			if header.Mode < 0 || header.Mode > 0o777 || header.Size < 0 {
				return fmt.Errorf("%w %q: invalid metadata", errArchiveEntry, header.Name)
			}

			permission := os.FileMode(header.Mode) & archiveFilePerm
			if err := extractFile(io.LimitReader(archive, header.Size), target, permission); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w %q", errArchiveEntry, header.Name)
		}
	}
}

func extractFile(source io.Reader, target string, permission os.FileMode) (returnErr error) {
	if err := os.MkdirAll(filepath.Dir(target), privateDirPerm); err != nil {
		return err
	}

	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, permission)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, file.Close()) }()

	_, err = io.Copy(file, source)

	return err
}
