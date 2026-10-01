//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package workloadcatalog

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func lockBuild(root, digest string) (func() error, error) {
	locks := filepath.Join(root, ".locks")
	if err := os.MkdirAll(locks, dirPerm); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filepath.Join(locks, digest+".lock"), os.O_CREATE|os.O_RDWR, filePerm)
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
