//go:build aix || solaris

package workloadcatalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func (store *Store) lockExclusive() (func() error, error) {
	if err := os.MkdirAll(store.root, dirPerm); err != nil {
		return nil, fmt.Errorf("create workload catalog lock directory: %w", err)
	}

	file, err := os.OpenFile(filepath.Join(store.root, ".lock"), os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return nil, fmt.Errorf("open workload catalog lock: %w", err)
	}

	lock := unix.Flock_t{Type: unix.F_WRLCK}
	if err := unix.FcntlFlock(file.Fd(), unix.F_SETLKW, &lock); err != nil {
		_ = file.Close()

		return nil, fmt.Errorf("lock workload catalog: %w", err)
	}

	return func() error {
		lock.Type = unix.F_UNLCK

		return errors.Join(unix.FcntlFlock(file.Fd(), unix.F_SETLKW, &lock), file.Close())
	}, nil
}
