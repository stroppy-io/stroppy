package csv

import (
	"context"
	stdcsv "encoding/csv"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	stroppyconfig "github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

// buildURL returns a URL string pointing at dir with the given query
// options. `workload=` is wired into every test so two parallel tests
// never collide on the output layout even when they share a tmp dir.
func buildURL(dir, workload string, extra map[string]string) string {
	q := url.Values{}
	q.Set("workload", workload)

	for k, v := range extra {
		q.Set(k, v)
	}

	return dir + "?" + q.Encode()
}

// newTestDriver builds a CSV driver rooted at a per-test tmp dir, with
// the given extra URL query options. Returns the driver plus the
// workload output directory the driver will write under.
func newTestDriver(t *testing.T, extra map[string]string) (*Driver, string) {
	t.Helper()

	root := t.TempDir()
	workload := "wl_" + strings.ReplaceAll(t.Name(), "/", "_")

	raw := buildURL(root, workload, extra)

	d, err := NewDriver(context.Background(), driver.Options{
		Config: &stroppyconfig.DriverConfig{Url: raw},
	})
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}

	return d, filepath.Join(d.cfg.dir, workload)
}

// readCSVFile returns every record in the CSV at path, including the
// header if present.
func readCSVFile(t *testing.T, path string) [][]string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %q: %v", path, err)
	}

	defer f.Close()

	rr := stdcsv.NewReader(f)
	rr.FieldsPerRecord = -1

	all, err := rr.ReadAll()
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	return all
}

func TestRunQueryAcceptsDDL(t *testing.T) {
	t.Parallel()

	d, _ := newTestDriver(t, nil)

	for _, q := range []string{
		"DROP TABLE foo",
		"drop table foo",
		"CREATE TABLE x (a int)",
		"TRUNCATE TABLE x",
		"COMMENT ON TABLE x IS 'hi'",
		"",
	} {
		if _, err := d.RunQuery(context.Background(), q, nil); err != nil {
			t.Fatalf("RunQuery(%q) err = %v", q, err)
		}
	}
}

func TestRunQueryRejectsNonDDL(t *testing.T) {
	t.Parallel()

	d, _ := newTestDriver(t, nil)

	_, err := d.RunQuery(context.Background(), "SELECT 1", nil)
	if !errors.Is(err, ErrCsvDriverNoQuery) {
		t.Fatalf("err = %v, want ErrCsvDriverNoQuery", err)
	}
}

func TestBeginRejected(t *testing.T) {
	t.Parallel()

	d, _ := newTestDriver(t, nil)

	if _, err := d.Begin(context.Background(), 0); !errors.Is(err, ErrCsvDriverNoQuery) {
		t.Fatalf("err = %v, want ErrCsvDriverNoQuery", err)
	}
}

func TestParseConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw   string
		dir   string
		sep   rune
		head  bool
		merge bool
		err   bool
	}{
		{raw: "/tmp/a", dir: "/tmp/a", sep: ',', head: true, merge: true},
		{raw: "/tmp/a?merge=false", dir: "/tmp/a", sep: ',', head: true, merge: false},
		{raw: "/tmp/a?separator=tab", dir: "/tmp/a", sep: '\t', head: true, merge: true},
		{raw: "/tmp/a?header=false", dir: "/tmp/a", sep: ',', head: false, merge: true},
		{raw: "/tmp/a?merge=bogus", err: true},
		{raw: "/tmp/a?separator=pipe", err: true},
	}

	for _, tc := range cases {
		cfg, err := parseConfig(tc.raw)
		if tc.err {
			if err == nil {
				t.Errorf("parseConfig(%q): expected error", tc.raw)
			}

			continue
		}

		if err != nil {
			t.Errorf("parseConfig(%q): %v", tc.raw, err)

			continue
		}

		if cfg.dir != tc.dir {
			t.Errorf("dir = %q, want %q", cfg.dir, tc.dir)
		}

		if cfg.separator != tc.sep {
			t.Errorf("sep = %q, want %q", cfg.separator, tc.sep)
		}

		if cfg.header != tc.head || cfg.merge != tc.merge {
			t.Errorf("flags: header=%v merge=%v, want header=%v merge=%v",
				cfg.header, cfg.merge, tc.head, tc.merge)
		}
	}
}

// TestManifestWritten verifies Teardown writes a MANIFEST.json recording
// every loaded table and its row count, via the typed Insert path.
func TestManifestWritten(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})

	if _, err := d.Insert(context.Background(), typedRowsReq("tm", 15, 1)); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	mp := filepath.Join(workDir, "MANIFEST.json")

	b, err := os.ReadFile(mp)
	if err != nil {
		t.Fatalf("read MANIFEST: %v", err)
	}

	if !strings.Contains(string(b), `"tm"`) {
		t.Fatalf("manifest missing table entry: %s", b)
	}

	if !strings.Contains(string(b), `"rows": 15`) {
		t.Fatalf("manifest missing row count: %s", b)
	}
}
