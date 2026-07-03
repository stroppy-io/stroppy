package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Eject writes a built-in test's full source (every embedded file, tests
// included, so the copy is a complete fork) into dir, plus a README explaining
// how to run it. It refuses a dir that already exists and is non-empty, so an
// eject never clobbers work. Returns the sorted list of written file names.
func Eject(name, dir string) ([]string, error) {
	src, err := Builtin(name)
	if err != nil {
		return nil, err
	}
	if err := ensureEmptyDir(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	written := make([]string, 0, len(src.Files)+1)
	for _, fn := range sortedNames(src.Files) {
		if err := os.WriteFile(filepath.Join(dir, fn), src.Files[fn], 0o644); err != nil {
			return nil, err
		}
		written = append(written, fn)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), ejectREADME(name), 0o644); err != nil {
		return nil, err
	}
	written = append(written, "README.md")
	sort.Strings(written)
	return written, nil
}

// ensureEmptyDir returns an error if dir exists and contains entries. A missing
// dir (or an existing empty one) is fine.
func ensureEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("refusing to eject into non-empty directory %s", dir)
	}
	return nil
}

// ejectREADME is the run guidance dropped next to an ejected test.
func ejectREADME(name string) []byte {
	return fmt.Appendf(nil, `# %s (ejected from stroppy2)

Run it through the CLI (materialises the embedded SDK, builds, and executes),
from this directory:

    stroppy2 run .

Or, once the SDK module is published, as a plain Go program:

    STROPPY_DRIVER_URL=postgres://user@host:5432/db go run .

Inspect it without running:

    stroppy2 probe .     # machine-readable description (JSON)
    stroppy2 plan  .     # step DAG
`, name)
}
