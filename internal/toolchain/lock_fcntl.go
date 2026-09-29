//go:build aix || solaris

package toolchain

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func lockInstall(root string) (func() error, error) {
	file, err := os.OpenFile(
		filepath.Join(root, ".install.lock"), os.O_CREATE|os.O_RDWR, privateFilePerm,
	)
	if err != nil {
		return nil, err
	}

	lock := unix.Flock_t{Type: unix.F_WRLCK}
	if err := unix.FcntlFlock(file.Fd(), unix.F_SETLKW, &lock); err != nil {
		_ = file.Close()

		return nil, err
	}

	return func() error {
		lock.Type = unix.F_UNLCK

		return errors.Join(unix.FcntlFlock(file.Fd(), unix.F_SETLKW, &lock), file.Close())
	}, nil
}
