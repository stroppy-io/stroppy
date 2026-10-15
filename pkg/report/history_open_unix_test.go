//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package report

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestHistoryOpenDoesNotBlockOnFIFO(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	name := "2026-10-01T00-00-00Z-fifo.json"
	if err := syscall.Mkfifo(filepath.Join(directory, name), 0o600); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	file, err := openHistoryReport(root, name)
	if err != nil {
		t.Fatal(err)
	}

	file.Close()
}
