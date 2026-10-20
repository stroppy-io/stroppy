//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package report

import "os"

func openHistoryReport(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}
