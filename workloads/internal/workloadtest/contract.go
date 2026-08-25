// Package workloadtest provides contract assertions for embedded workload assets.
package workloadtest

import (
	"io/fs"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

// Query identifies one required named query.
type Query struct {
	Section string
	Name    string
}

// Files asserts that every named asset exists.
func Files(t *testing.T, files fs.FS, names ...string) {
	t.Helper()

	for _, name := range names {
		if _, err := fs.Stat(files, name); err != nil {
			t.Errorf("required asset %q: %v", name, err)
		}
	}
}

// SQL asserts required sections and named queries in one embedded SQL asset.
func SQL(t *testing.T, files fs.FS, name string, sections []string, queries []Query) {
	t.Helper()

	data, err := fs.ReadFile(files, name)
	if err != nil {
		t.Fatalf("read %q: %v", name, err)
	}

	parsed := bench.ParseSQL(string(data))
	for _, section := range sections {
		if len(parsed.Section(section)) == 0 {
			t.Errorf("%s: missing or empty section %q", name, section)
		}
	}

	for _, query := range queries {
		if body, ok := parsed.Query(query.Section, query.Name); !ok || body == "" {
			t.Errorf("%s: missing or empty query %s/%s", name, query.Section, query.Name)
		}
	}
}
