//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package workloadcatalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func (store *Store) lockExclusive() (func() error, error) {
	if err := os.MkdirAll(store.root, dirPerm); err != nil {
		return nil, fmt.Errorf("create workload catalog lock directory: %w", err)
	}

	file, err := os.OpenFile(filepath.Join(store.root, ".lock"), os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return nil, fmt.Errorf("open workload catalog lock: %w", err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()

		return nil, fmt.Errorf("lock workload catalog: %w", err)
	}

	unlock := func() error {
		return errors.Join(
			syscall.Flock(int(file.Fd()), syscall.LOCK_UN),
			file.Close(),
		)
	}

	return unlock, nil
}
