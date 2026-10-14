package report

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// SameOutput reports whether two paths publish to the same directory entry.
// Existing identities also catch case-insensitive names and hard-link aliases.
func SameOutput(a, b string) (bool, error) {
	first, firstErr := outputIdentity(a)

	second, secondErr := outputIdentity(b)
	if err := errors.Join(firstErr, secondErr); err != nil {
		return false, err
	}

	if first == second {
		return true, nil
	}

	firstInfo, firstErr := os.Stat(a)

	secondInfo, secondErr := os.Stat(b)
	if firstErr == nil && secondErr == nil {
		return os.SameFile(firstInfo, secondInfo), nil
	}

	firstParent, firstErr := os.Stat(filepath.Dir(first))

	secondParent, secondErr := os.Stat(filepath.Dir(second))
	if firstErr == nil && secondErr == nil && os.SameFile(firstParent, secondParent) &&
		strings.EqualFold(filepath.Base(first), filepath.Base(second)) {
		return caseInsensitiveDirectory(filepath.Dir(first))
	}

	return false, nil
}

func caseInsensitiveDirectory(directory string) (insensitive bool, err error) {
	probe, err := os.CreateTemp(directory, ".stroppy-case-*")
	if err != nil {
		return false, err
	}

	name := probe.Name()
	defer func() { err = errors.Join(err, probe.Close(), os.Remove(name)) }()

	original, err := probe.Stat()
	if err != nil {
		return false, err
	}

	upper, err := os.Stat(filepath.Join(directory, strings.ToUpper(filepath.Base(name))))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return os.SameFile(original, upper), nil
}

func outputIdentity(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if errors.Is(err, os.ErrNotExist) {
		return absolute, nil
	}

	if err != nil {
		return "", err
	}

	return filepath.Join(parent, filepath.Base(absolute)), nil
}
