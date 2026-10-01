//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package workloadcatalog

import "errors"

func lockBuild(_, _ string) (func() error, error) {
	return nil, errors.New("build-cache locking is unsupported on this platform")
}
