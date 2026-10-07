package author

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const (
	projectDirectoryMode = 0o755
	projectFileMode      = 0o644
)

var ErrDestination = errors.New("destination must be a new or empty real directory")

type ownedDirectory struct {
	name string
	info fs.FileInfo
}

func removeOwnedDirectory(root *os.Root, directory ownedDirectory) {
	current, err := root.Lstat(directory.name)
	if err == nil && current.IsDir() && os.SameFile(directory.info, current) {
		_ = root.Remove(directory.name)
	}
}

func removeOwnedRoot(destination string, identity fs.FileInfo) {
	current, err := os.Lstat(destination)
	if err == nil && current.IsDir() && os.SameFile(identity, current) {
		_ = os.Remove(destination)
	}
}

// Write creates files exclusively beneath an opened destination directory.
// Rollback removes only unchanged files created by this operation.
//
//nolint:gocognit,gocyclo,cyclop,funlen,nestif,maintidx // exclusive file creation and owned-file rollback.
func Write(destination string, files map[string][]byte) (returnErr error) {
	destination = filepath.Clean(destination)

	for name := range files {
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:") {
			return fmt.Errorf("%w: path %q", ErrInvalidSource, name)
		}

		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := files[parent]; exists {
				return fmt.Errorf("%w: file/directory collision %q", ErrInvalidSource, parent)
			}
		}
	}

	created := false
	if err := os.Mkdir(destination, projectDirectoryMode); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}

	info, err := os.Lstat(destination)
	if err != nil {
		return err
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrDestination
	}

	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()

	openedInfo, err := root.Stat(".")
	if err != nil {
		return err
	}

	if !os.SameFile(info, openedInfo) {
		return ErrDestination
	}

	directory, err := root.Open(".")
	if err == nil {
		current, identityErr := os.Lstat(destination)
		if identityErr != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedInfo, current) {
			_ = directory.Close()

			return ErrDestination
		}
	}

	if err != nil {
		return err
	}

	entries, err := directory.Readdirnames(1)
	closeErr := directory.Close()

	if err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, io.EOF) {
		return err
	}

	if closeErr != nil {
		return closeErr
	}

	if len(entries) != 0 {
		return ErrDestination
	}

	owned := map[string]fs.FileInfo{}
	directories := []ownedDirectory{}

	defer func() {
		if returnErr == nil {
			return
		}

		for name, identity := range owned {
			current, err := root.Lstat(name)
			if err == nil && os.SameFile(identity, current) && current.Size() == identity.Size() &&
				current.ModTime().Equal(identity.ModTime()) {
				_ = root.Remove(name)
			}
		}

		for i := len(directories) - 1; i >= 0; i-- {
			removeOwnedDirectory(root, directories[i])
		}

		if created {
			removeOwnedRoot(destination, openedInfo)
		}
	}()

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}

	slices.Sort(names)

	for _, name := range names {
		components := strings.Split(path.Dir(name), "/")
		parent := ""

		for _, component := range components {
			if component == "." {
				continue
			}

			parent = path.Join(parent, component)
			if err := root.Mkdir(parent, projectDirectoryMode); err == nil {
				parentInfo, err := root.Lstat(parent)
				if err != nil {
					return err
				}

				if !parentInfo.IsDir() {
					return ErrDestination
				}

				directories = append(directories, ownedDirectory{parent, parentInfo})
			} else if !errors.Is(err, os.ErrExist) {
				return err
			} else {
				index := slices.IndexFunc(directories, func(entry ownedDirectory) bool { return entry.name == parent })
				if index < 0 {
					return ErrDestination
				}

				parentInfo, err := root.Lstat(parent)
				if err != nil {
					return err
				}

				if !parentInfo.IsDir() || !os.SameFile(directories[index].info, parentInfo) {
					return ErrDestination
				}
			}
		}

		file, err := root.OpenFile(filepath.FromSlash(name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, projectFileMode)
		if err != nil {
			return err
		}

		identity, err := file.Stat()
		if err != nil {
			_ = file.Close()

			return err
		}

		owned[name] = identity

		_, writeErr := file.Write(files[name])

		finalIdentity, statErr := file.Stat()
		if statErr == nil {
			owned[name] = finalIdentity
		}

		if err := errors.Join(writeErr, statErr, file.Close()); err != nil {
			return err
		}
	}

	return nil
}
