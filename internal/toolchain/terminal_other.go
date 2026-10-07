//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package toolchain

import "io"

func interactive(_ io.Reader, _ io.Writer) bool {
	return false
}
