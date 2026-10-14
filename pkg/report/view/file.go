package view

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// WriteFile atomically publishes a standalone HTML report.
func WriteFile(path string, data []byte, id string) error {
	return WriteCaptureFile(path, data, id, ".")
}

// WriteCaptureFile resolves sidecar streams against their report directory.
func WriteCaptureFile(path string, data []byte, id, base string) (err error) {
	capture, err := report.CaptureFromJSON(data)
	if err != nil {
		return err
	}

	if same, err := report.OutputOverwritesCapture(path, base, capture); err != nil {
		return err
	} else if same {
		return report.ErrCaptureArtifact
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".stroppy-html-*.tmp")
	if err != nil {
		return fmt.Errorf("create HTML report: %w", err)
	}

	name := file.Name()

	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close(), os.Remove(name))
		}
	}()

	if err := RenderCapture(file, data, id, base, nil); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(name, path)
}

// Export writes HTML to output when path is empty, otherwise to an atomic file.
func Export(output io.Writer, path string, data []byte, id string) error {
	if path != "" {
		return WriteFile(path, data, id)
	}

	return Render(output, data, id)
}
