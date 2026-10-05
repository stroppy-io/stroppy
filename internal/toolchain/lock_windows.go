//go:build windows

package toolchain

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func lockInstall(root string) (func() error, error) {
	file, err := os.OpenFile(
		filepath.Join(root, ".install.lock"), os.O_CREATE|os.O_RDWR, privateFilePerm,
	)
	if err != nil {
		return nil, err
	}

	handle := windows.Handle(file.Fd())
	overlapped := &windows.Overlapped{}
	if err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		overlapped,
	); err != nil {
		_ = file.Close()

		return nil, err
	}

	return func() error {
		return errors.Join(windows.UnlockFileEx(handle, 0, 1, 0, overlapped), file.Close())
	}, nil
}
