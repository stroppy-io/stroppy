//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package workloadcatalog

import "errors"

func (store *Store) lockExclusive() (func() error, error) {
	return nil, errors.New("workload catalog locking is unsupported on this platform")
}
