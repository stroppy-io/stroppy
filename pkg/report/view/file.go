package view

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteFile atomically publishes a standalone HTML report.
func WriteFile(path string, data []byte, id string) (err error) {
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

	if err := Render(file, data, id); err != nil {
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
