package workloadcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "\x00\r\n") {
		return fmt.Errorf("%w: bad workload name %q", ErrInvalidEntry, name)
	}

	return nil
}

func entryID(name string) string {
	hash := sha256.Sum256([]byte(name))

	return hex.EncodeToString(hash[:])
}

func copyStaged(source, destinationDir, prefix string, permission os.FileMode) (path string, returnErr error) {
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}

	defer input.Close()

	if err := os.MkdirAll(destinationDir, dirPerm); err != nil {
		return "", err
	}

	temporary, err := os.CreateTemp(destinationDir, prefix)
	if err != nil {
		return "", err
	}

	temporaryPath := temporary.Name()

	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, temporary.Close(), os.Remove(temporaryPath))
		}
	}()

	if _, err := io.Copy(temporary, input); err != nil {
		return "", err
	}

	if err := temporary.Chmod(permission); err != nil {
		return "", err
	}

	if err := temporary.Sync(); err != nil {
		return "", err
	}

	if err := temporary.Close(); err != nil {
		return "", err
	}

	return temporaryPath, nil
}

func writeAtomic(path string, data []byte, permission os.FileMode) (returnErr error) {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), ".stroppy-entry-*.tmp")
	if err != nil {
		return err
	}

	temporaryPath := temporary.Name()

	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, temporary.Close(), os.Remove(temporaryPath))
		}
	}()

	if _, err := temporary.Write(data); err != nil {
		return err
	}

	if err := temporary.Chmod(permission); err != nil {
		return err
	}

	if err := temporary.Sync(); err != nil {
		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, path)
}
