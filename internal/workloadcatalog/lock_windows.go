//go:build windows

package workloadcatalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func (store *Store) lockExclusive() (func() error, error) {
	if err := os.MkdirAll(store.root, dirPerm); err != nil {
		return nil, fmt.Errorf("create workload catalog lock directory: %w", err)
	}

	name := ".lock"
	if store.lockKey != "" && store.lockKey != "catalog" {
		locks := filepath.Join(store.root, ".locks")
		if err := os.MkdirAll(locks, dirPerm); err != nil {
			return nil, fmt.Errorf("create workload lock directory: %w", err)
		}
		name = filepath.Join(".locks", store.lockKey+".lock")
	}

	file, err := os.OpenFile(filepath.Join(store.root, name), os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return nil, fmt.Errorf("open workload catalog lock: %w", err)
	}

	handle := windows.Handle(file.Fd())
	overlapped := &windows.Overlapped{}
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped); err != nil {
		_ = file.Close()

		return nil, fmt.Errorf("lock workload catalog: %w", err)
	}

	return func() error {
		return errors.Join(windows.UnlockFileEx(handle, 0, 1, 0, overlapped), file.Close())
	}, nil
}
