//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package toolchain

import "errors"

func lockInstall(_ string) (func() error, error) {
	return nil, errors.New("private Go toolchain installation is unsupported on this host")
}
