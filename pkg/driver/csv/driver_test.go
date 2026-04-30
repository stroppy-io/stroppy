package csv

import (
	"context"
	stdcsv "encoding/csv"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
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
		Config: &stroppy.DriverConfig{Url: raw},
	})
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}

	return d, filepath.Join(d.cfg.dir, workload)
}

// litInt / rowIndex / binOp mirror the proto builders used by the
// noop driver test. They stay local so the csv package has zero
// test-time coupling to runtime internals.
func litInt(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
}

func litStr(s string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_String_{String_: s},
	}}}
}

func rowIndex() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_GLOBAL,
	}}}
}

func binOp(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: op, A: a, B: b,
	}}}
}

func rowsSpec(table string, size int64, workers int32) *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		{Name: "id", Expr: binOp(dgproto.BinOp_ADD, rowIndex(), litInt(1))},
		{Name: "squared", Expr: binOp(dgproto.BinOp_MUL, rowIndex(), rowIndex())},
		{Name: "label", Expr: litStr("row")},
	}

	return &dgproto.InsertSpec{
		Table:       table,
		Method:      dgproto.InsertMethod_NATIVE,
		Parallelism: &dgproto.Parallelism{Workers: workers},
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: table, Size: size},
			Attrs:       attrs,
			ColumnOrder: []string{"id", "squared", "label"},
		},
	}
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

func TestInsertSpecSingleShardMerge(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})

	sp := rowsSpec("t1", 100, 0)

	stat, err := d.InsertSpec(context.Background(), sp)
	if err != nil {
		t.Fatalf("InsertSpec: %v", err)
	}

	if stat.Rows != 100 {
		t.Fatalf("InsertSpec rows = %d, want 100", stat.Rows)
	}

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	out := filepath.Join(workDir, "t1.csv")

	records := readCSVFile(t, out)
	if len(records) != 101 {
		t.Fatalf("records = %d, want 101 (header + 100)", len(records))
	}

	header := records[0]
	if header[0] != "id" || header[1] != "squared" || header[2] != "label" {
		t.Fatalf("header = %v, want [id squared label]", header)
	}

	// Random-access row check.
	row42 := records[43]

	got, _ := strconv.ParseInt(row42[0], 10, 64)
	if got != 43 {
		t.Fatalf("records[43][0] = %d, want 43", got)
	}

	// .shards/ must be cleaned up by the merge pass.
	if _, err := os.Stat(filepath.Join(workDir, ".shards")); !os.IsNotExist(err) {
		t.Fatalf(".shards dir still present after merge: %v", err)
	}
}

func TestInsertSpecParallelMerge(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})

	const total int64 = 4000

	sp := rowsSpec("t_multi", total, 4)

	stat, err := d.InsertSpec(context.Background(), sp)
	if err != nil {
		t.Fatalf("InsertSpec: %v", err)
	}

	if stat.Rows != total {
		t.Fatalf("InsertSpec rows = %d, want %d", stat.Rows, total)
	}

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	out := filepath.Join(workDir, "t_multi.csv")

	records := readCSVFile(t, out)
	if int64(len(records)-1) != total {
		t.Fatalf("records - header = %d, want %d", len(records)-1, total)
	}

	ids := make(map[int64]struct{}, total)

	for _, row := range records[1:] {
		v, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil {
			t.Fatalf("parse id %q: %v", row[0], err)
		}

		ids[v] = struct{}{}
	}

	if int64(len(ids)) != total {
		t.Fatalf("unique ids = %d, want %d", len(ids), total)
	}
}

func TestInsertSpecShardsNoMerge(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "false"})

	sp := rowsSpec("t_no_merge", 250, 3)

	stat, err := d.InsertSpec(context.Background(), sp)
	if err != nil {
		t.Fatalf("InsertSpec: %v", err)
	}

	if stat.Rows != 250 {
		t.Fatalf("InsertSpec rows = %d, want 250", stat.Rows)
	}

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(workDir, "t_no_merge.w*.csv"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	if len(matches) != 3 {
		t.Fatalf("shards = %d, want 3", len(matches))
	}

	// Shards have no header rows — count must equal the row count.
	var total int

	for _, m := range matches {
		total += len(readCSVFile(t, m))
	}

	if total != 250 {
		t.Fatalf("rows across shards = %d, want 250", total)
	}

	// Sidecar header must be present.
	header := readCSVFile(t, filepath.Join(workDir, "t_no_merge.header.csv"))
	if len(header) != 1 || header[0][0] != "id" {
		t.Fatalf("header sidecar = %v", header)
	}
}

func TestInsertSpecDeterminismAcrossWorkers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	snapshots := make([][]string, 0, 3)

	for _, workers := range []int32{1, 4, 16} {
		dir := t.TempDir()
		workload := "det_" + strconv.Itoa(int(workers))
		raw := buildURL(dir, workload, map[string]string{"merge": "true"})

		d, err := NewDriver(ctx, driver.Options{Config: &stroppy.DriverConfig{Url: raw}})
		if err != nil {
			t.Fatalf("NewDriver: %v", err)
		}

		const total int64 = 2000

		sp := rowsSpec("t_det", total, workers)

		if _, err := d.InsertSpec(ctx, sp); err != nil {
			t.Fatalf("InsertSpec(workers=%d): %v", workers, err)
		}

		if err := d.Teardown(ctx); err != nil {
			t.Fatalf("Teardown(workers=%d): %v", workers, err)
		}

		out := filepath.Join(dir, workload, "t_det.csv")

		records := readCSVFile(t, out)
		if int64(len(records)-1) != total {
			t.Fatalf("records - header = %d, want %d at workers=%d",
				len(records)-1, total, workers)
		}

		body := make([]string, 0, total)
		for _, rec := range records[1:] {
			body = append(body, strings.Join(rec, "|"))
		}

		sort.Strings(body)

		snapshots = append(snapshots, body)
	}

	// workers ∈ {1, 4, 16} → identical sorted multisets.
	for i := 1; i < len(snapshots); i++ {
		if strings.Join(snapshots[0], "\n") != strings.Join(snapshots[i], "\n") {
			t.Fatalf("determinism violated at snapshot index %d", i)
		}
	}
}

func TestInsertSpecRejectsNonNative(t *testing.T) {
	t.Parallel()

	d, _ := newTestDriver(t, nil)

	sp := rowsSpec("t_bad", 10, 0)
	sp.Method = dgproto.InsertMethod_PLAIN_BULK

	_, err := d.InsertSpec(context.Background(), sp)
	if !errors.Is(err, ErrUnsupportedInsertMethod) {
		t.Fatalf("err = %v, want ErrUnsupportedInsertMethod", err)
	}
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

func TestManifestWritten(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})

	sp := rowsSpec("tm", 15, 0)

	if _, err := d.InsertSpec(context.Background(), sp); err != nil {
		t.Fatalf("InsertSpec: %v", err)
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
