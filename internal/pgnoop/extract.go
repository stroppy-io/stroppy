package pgnoop

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/ulikunitz/xz"
)

var errBinaryNotInTarball = errors.New("binary not found in tarball")

// extractTarXz decompresses an xz stream and returns the file inside the tar
// archive whose base name matches wanted.
func extractTarXz(tarball []byte, wanted string) ([]byte, error) {
	xzReader, err := xz.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return nil, fmt.Errorf("pgnoop: open xz stream: %w", err)
	}

	tarReader := tar.NewReader(xzReader)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("pgnoop: read tarball: %w", err)
		}

		if header.Typeflag != tar.TypeReg || path.Base(header.Name) != wanted {
			continue
		}

		data, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("pgnoop: read %q from tarball: %w", header.Name, err)
		}

		return data, nil
	}

	return nil, fmt.Errorf("%w: %q", errBinaryNotInTarball, wanted)
}
