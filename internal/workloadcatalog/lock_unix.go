//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

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
