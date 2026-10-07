//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package toolchain

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func lockInstall(root string) (func() error, error) {
	file, err := os.OpenFile(
		filepath.Join(root, ".install.lock"), os.O_CREATE|os.O_RDWR, privateFilePerm,
	)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()

		return nil, err
	}

	return func() error {
		return errors.Join(syscall.Flock(int(file.Fd()), syscall.LOCK_UN), file.Close())
	}, nil
}
