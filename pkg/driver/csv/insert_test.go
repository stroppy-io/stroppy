package csv

import (
	"bytes"
	"context"
	stdcsv "encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	stroppyconfig "github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// typedRowsSource returns a 3-column indexed source (id, squared, label)
// over `total` rows whose values match the legacy rowsSpec: id=entity+1,
// squared=entity*entity, label="row". Used to exercise the typed Insert
// path against the same expected CSV output as the InsertSpec tests.
func typedRowsSource(total int64) *gen.IndexedSource {
	return typedRowsSourceWithFailure(total, -1)
}

func typedIDsSource(total int64) *gen.IndexedSource {
	b := gen.NewSchemaBuilder()
	idCol := b.Int64("id")

	return gen.NewIndexedSource(b.Build(), gen.Root{}, "test/csv-ids@1", total, 64,
		func(r gen.Row, entity uint64) error {
			r.SetInt64(idCol, int64(entity)+1)

			return nil
		})
}

var errInjectedSource = errors.New("injected source failure")

func typedRowsSourceWithFailure(total, failAt int64) *gen.IndexedSource {
	b := gen.NewSchemaBuilder()
	idCol := b.Int64("id")
	sqCol := b.Int64("squared")
	labelCol := b.Bytes("label", 3)
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		if int64(entity) == failAt {
			return errInjectedSource
		}

		r.SetInt64(idCol, int64(entity)+1)
		r.SetInt64(sqCol, int64(entity)*int64(entity))

		dst, err := r.Bytes(labelCol, 3)
		if err != nil {
			return err
		}

		copy(dst, "row")

		return nil
	}

	return gen.NewIndexedSource(schema, gen.Root{}, "test/csv@1", total, 64, fn)
}

func typedRowsReq(table string, total int64, workers int) *driver.InsertRequest {
	return &driver.InsertRequest{
		Table:   table,
		Method:  driver.InsertNative,
		Workers: workers,
		Source:  typedRowsSource(total),
	}
}

func readManifest(t *testing.T, workDir string) manifest {
	t.Helper()

	blob, err := os.ReadFile(filepath.Join(workDir, manifestFilename))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var doc manifest
	if err := json.Unmarshal(blob, &doc); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	return doc
}

func reopenTestDriver(t *testing.T, previous *Driver, extra map[string]string) *Driver {
	t.Helper()

	d, err := NewDriver(context.Background(), driver.Options{
		Config: &stroppyconfig.DriverConfig{URL: buildURL(previous.cfg.dir, previous.cfg.workload, extra)},
	})
	if err != nil {
		t.Fatalf("reopen driver: %v", err)
	}

	return d
}

func TestInsertRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()

	d, _ := newTestDriver(t, nil)
	req := typedRowsReq("unsupported", 1, 1)
	req.Method = driver.InsertPlainQuery

	_, err := d.Insert(context.Background(), req)
	if !errors.Is(err, driver.ErrInsertMethodNotSupported) || !errors.Is(err, ErrUnsupportedInsertMethod) {
		t.Fatalf("Insert error = %v, want CSV and driver unsupported-method errors", err)
	}

	if facts := d.ClassifyError(err); facts.Kind != driver.ErrorKindUnsupported {
		t.Fatalf("ClassifyError() = %#v, want unsupported", facts)
	}
}

func TestInsertSingleShardMerge(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})

	stat, err := d.Insert(context.Background(), typedRowsReq("t1", 100, 1))
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if stat.Rows != 100 {
		t.Fatalf("Insert rows = %d, want 100", stat.Rows)
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

	// Random-access row check: records[43] is entity 42 → id=43, squared=1764.
	row42 := records[43]

	got, _ := strconv.ParseInt(row42[0], 10, 64)
	if got != 43 {
		t.Fatalf("records[43][0] = %d, want 43", got)
	}

	sq, _ := strconv.ParseInt(row42[1], 10, 64)
	if sq != 1764 {
		t.Fatalf("records[43][1] = %d, want 1764", sq)
	}

	if row42[2] != "row" {
		t.Fatalf("records[43][2] = %q, want row", row42[2])
	}

	// Successful finalization publishes both artifacts, removes shards, and
	// leaves no temporary output behind.
	if _, err := os.Stat(filepath.Join(workDir, "MANIFEST.json")); err != nil {
		t.Fatalf("MANIFEST.json missing after merge: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, ".shards")); !os.IsNotExist(err) {
		t.Fatalf(".shards dir still present after merge: %v", err)
	}

	assertNoTemporaryOutputs(t, workDir)
}

func TestInsertParallelMerge(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})

	const total int64 = 4000

	stat, err := d.Insert(context.Background(), typedRowsReq("t_multi", total, 4))
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if stat.Rows != total {
		t.Fatalf("Insert rows = %d, want %d", stat.Rows, total)
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

	for _, rec := range records[1:] {
		id, _ := strconv.ParseInt(rec[0], 10, 64)
		ids[id] = struct{}{}
	}

	if int64(len(ids)) != total {
		t.Fatalf("distinct ids = %d, want %d", len(ids), total)
	}

	// No id repeats across worker shards (worker-invariance + no double-write).
	for _, rec := range records[1:] {
		if rec[2] != "row" {
			t.Fatalf("label = %q, want row", rec[2])
		}
	}
}

func TestRepeatedInsertsUseDistinctShards(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})
	if _, err := d.Insert(context.Background(), typedRowsReq("repeated", 3, 1)); err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	if _, err := d.Insert(context.Background(), typedRowsReq("repeated", 4, 2)); err != nil {
		t.Fatalf("second Insert: %v", err)
	}

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if got := len(readCSVFile(t, filepath.Join(workDir, "repeated.csv"))) - 1; got != 7 {
		t.Fatalf("merged rows = %d, want 7", got)
	}

	table := readManifest(t, workDir).Tables["repeated"]
	if table.Rows != 7 || table.Shards != 3 {
		t.Fatalf("manifest table = %#v, want 7 rows and 3 shards", table)
	}
}

func TestNewGenerationInvalidatesManifestAndClearsStaleShards(t *testing.T) {
	t.Parallel()

	firstOptions := map[string]string{"merge": "false"}
	secondOptions := map[string]string{"merge": "true"}

	first, workDir := newTestDriver(t, firstOptions)
	if _, err := first.Insert(context.Background(), typedRowsReq("replace", 8, 4)); err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	if err := first.Teardown(context.Background()); err != nil {
		t.Fatalf("first Teardown: %v", err)
	}

	second := reopenTestDriver(t, first, secondOptions)
	if _, err := second.Insert(context.Background(), typedRowsReq("replace", 2, 1)); err != nil {
		t.Fatalf("replacement Insert: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, manifestFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prior manifest remained during replacement: %v", err)
	}

	staleShards, err := shardFiles(workDir, "replace")
	if err != nil {
		t.Fatalf("list prior shards: %v", err)
	}

	if len(staleShards) != 0 {
		t.Fatalf("stale unmerged shards remained: %v", staleShards)
	}

	if _, err := os.Stat(filepath.Join(workDir, "replace.header.csv")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale header sidecar remained: %v", err)
	}

	shards, err := shardFiles(filepath.Join(workDir, ".shards"), "replace")
	if err != nil {
		t.Fatalf("list replacement shards: %v", err)
	}

	if len(shards) != 1 {
		t.Fatalf("replacement shards = %v, want one", shards)
	}

	if err := second.Teardown(context.Background()); err != nil {
		t.Fatalf("replacement Teardown: %v", err)
	}

	doc := readManifest(t, workDir)
	if !doc.Config.Merge {
		t.Fatal("replacement manifest did not record merged layout")
	}

	table := doc.Tables["replace"]
	if table.Rows != 2 || table.Shards != 1 {
		t.Fatalf("replacement manifest table = %#v, want 2 rows and 1 shard", table)
	}
}

func TestFailedReplacementWithholdsPriorManifest(t *testing.T) {
	t.Parallel()

	options := map[string]string{"merge": "false"}

	first, workDir := newTestDriver(t, options)
	if _, err := first.Insert(context.Background(), typedRowsReq("replace", 10, 1)); err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	if err := first.Teardown(context.Background()); err != nil {
		t.Fatalf("first Teardown: %v", err)
	}

	second := reopenTestDriver(t, first, options)
	failed := typedRowsReq("replace", 100, 1)
	failed.Source = typedRowsSourceWithFailure(100, 50)

	if _, err := second.Insert(context.Background(), failed); !errors.Is(err, errInjectedSource) {
		t.Fatalf("replacement Insert error = %v, want injected source failure", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, manifestFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed replacement exposed prior manifest: %v", err)
	}

	if err := second.Teardown(context.Background()); !errors.Is(err, ErrIncompleteLoad) {
		t.Fatalf("replacement Teardown error = %v, want ErrIncompleteLoad", err)
	}
}

func TestTeardownRejectsShardCountMismatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		merge  string
		mutate func(*testing.T, string, []string)
	}{
		{
			name:  "missing merged shard",
			merge: "true",
			mutate: func(t *testing.T, _ string, shards []string) {
				t.Helper()

				if err := os.Remove(shards[0]); err != nil {
					t.Fatalf("remove shard: %v", err)
				}
			},
		},
		{
			name:  "extra unmerged shard",
			merge: "false",
			mutate: func(t *testing.T, workDir string, _ []string) {
				t.Helper()

				if err := os.WriteFile(filepath.Join(workDir, "count.w999.csv"), []byte("extra\n"), fileMode); err != nil {
					t.Fatalf("write extra shard: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, workDir := newTestDriver(t, map[string]string{"merge": tc.merge})
			if _, err := d.Insert(context.Background(), typedRowsReq("count", 10, 2)); err != nil {
				t.Fatalf("Insert: %v", err)
			}

			shardDir := workDir
			if d.cfg.merge {
				shardDir = filepath.Join(workDir, ".shards")
			}

			shards, err := shardFiles(shardDir, "count")
			if err != nil {
				t.Fatalf("list shards: %v", err)
			}

			tc.mutate(t, workDir, shards)

			if err := d.Teardown(context.Background()); !errors.Is(err, ErrIncompleteLoad) {
				t.Fatalf("Teardown error = %v, want ErrIncompleteLoad", err)
			}

			if _, err := os.Stat(filepath.Join(workDir, manifestFilename)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("shard mismatch exposed manifest: %v", err)
			}
		})
	}
}

func TestRepeatedInsertRejectsColumnChange(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})
	if _, err := d.Insert(context.Background(), typedRowsReq("columns", 2, 1)); err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	changed := typedRowsReq("columns", 2, 1)
	changed.Source = typedIDsSource(2)

	_, err := d.Insert(context.Background(), changed)
	if err == nil || !strings.Contains(err.Error(), "column layout changed") {
		t.Fatalf("changed-column Insert error = %v", err)
	}

	if err := d.Teardown(context.Background()); !errors.Is(err, ErrIncompleteLoad) {
		t.Fatalf("Teardown error = %v, want ErrIncompleteLoad", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, manifestFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("changed-column load exposed manifest: %v", err)
	}
}

func TestDropResetsFailedGeneration(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})
	failed := typedRowsReq("reset", 100, 1)
	failed.Source = typedRowsSourceWithFailure(100, 50)

	if _, err := d.Insert(context.Background(), failed); !errors.Is(err, errInjectedSource) {
		t.Fatalf("failed Insert error = %v, want injected source failure", err)
	}

	if _, err := d.RunQuery(context.Background(), "DROP TABLE reset", nil); err != nil {
		t.Fatalf("DROP reset: %v", err)
	}

	if _, err := d.Insert(context.Background(), typedRowsReq("reset", 2, 1)); err != nil {
		t.Fatalf("replacement Insert: %v", err)
	}

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("replacement Teardown: %v", err)
	}

	table := readManifest(t, workDir).Tables["reset"]
	if table.Rows != 2 || table.Shards != 1 {
		t.Fatalf("replacement manifest table = %#v, want 2 rows and 1 shard", table)
	}
}

func TestInsertFailurePreventsFinalization(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})
	if _, err := d.Insert(context.Background(), typedRowsReq("complete", 4000, 4)); err != nil {
		t.Fatalf("complete Insert: %v", err)
	}

	failed := typedRowsReq("partial", 10_000, 1)

	failed.Source = typedRowsSourceWithFailure(10_000, 5000)

	if _, err := d.Insert(context.Background(), failed); !errors.Is(err, errInjectedSource) {
		t.Fatalf("partial Insert error = %v, want injected source failure", err)
	}

	if err := d.Teardown(context.Background()); !errors.Is(err, ErrIncompleteLoad) {
		t.Fatalf("detached Teardown error = %v, want ErrIncompleteLoad", err)
	}

	for _, name := range []string{"complete.csv", "partial.csv", "MANIFEST.json"} {
		if _, err := os.Stat(filepath.Join(workDir, name)); !os.IsNotExist(err) {
			t.Errorf("failed load published %s: %v", name, err)
		}
	}

	completed, err := filepath.Glob(filepath.Join(workDir, ".shards", "complete.w*.csv"))
	if err != nil {
		t.Fatalf("glob completed shards: %v", err)
	}

	if len(completed) != 4 {
		t.Fatalf("recoverable completed shards = %v, want 4", completed)
	}

	partial, err := filepath.Glob(filepath.Join(workDir, ".shards", "partial.w*.csv"))
	if err != nil {
		t.Fatalf("glob partial shards: %v", err)
	}

	if len(partial) != 0 {
		t.Fatalf("partial shard was committed: %v", partial)
	}

	assertNoTemporaryOutputs(t, workDir)
}

func TestManifestFailurePreservesShards(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})
	if _, err := d.Insert(context.Background(), typedRowsReq("manifest_failure", 100, 1)); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	manifestPath := filepath.Join(workDir, "MANIFEST.json")
	if err := os.Mkdir(manifestPath, dirMode); err != nil {
		t.Fatalf("create manifest publication blocker: %v", err)
	}

	if err := d.Teardown(context.Background()); err == nil {
		t.Fatal("Teardown succeeded despite manifest publication blocker")
	}

	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatalf("stat manifest blocker: %v", err)
	}

	if !info.IsDir() {
		t.Fatalf("manifest blocker became a completed file: mode=%s", info.Mode())
	}

	shards, err := filepath.Glob(filepath.Join(workDir, ".shards", "manifest_failure.w*.csv"))
	if err != nil {
		t.Fatalf("glob shards: %v", err)
	}

	if len(shards) != 1 {
		t.Fatalf("recoverable shards = %v, want one shard", shards)
	}

	assertNoTemporaryOutputs(t, workDir)

	if err := os.Remove(manifestPath); err != nil {
		t.Fatalf("remove manifest blocker: %v", err)
	}

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("retry Teardown: %v", err)
	}

	manifestInfo, err := os.Stat(manifestPath)
	if err != nil || !manifestInfo.Mode().IsRegular() {
		t.Fatalf("retry did not publish regular manifest: info=%v err=%v", manifestInfo, err)
	}

	if _, err := os.Stat(filepath.Join(workDir, ".shards")); !os.IsNotExist(err) {
		t.Fatalf("retry did not clean shards: %v", err)
	}
}

func TestManifestCancellationPreservesShards(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})
	if _, err := d.Insert(context.Background(), typedRowsReq("manifest_cancel", 100, 1)); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := d.mergeAll(context.Background(), workDir, d.tables); err != nil {
		t.Fatalf("mergeAll: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := writeManifest(ctx, workDir, d.workloadName, d.cfg, d.tables); !errors.Is(err, context.Canceled) {
		t.Fatalf("writeManifest error = %v, want context.Canceled", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "MANIFEST.json")); !os.IsNotExist(err) {
		t.Fatalf("canceled publication exposed manifest: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, ".shards")); err != nil {
		t.Fatalf("canceled publication removed shards: %v", err)
	}

	assertNoTemporaryOutputs(t, workDir)

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("retry Teardown: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, ".shards")); !os.IsNotExist(err) {
		t.Fatalf("successful retry did not clean shards: %v", err)
	}
}

func TestTeardownPreCanceledPreservesShards(t *testing.T) {
	t.Parallel()

	d, workDir := newTestDriver(t, map[string]string{"merge": "true"})
	if _, err := d.Insert(context.Background(), typedRowsReq("canceled", 100, 1)); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := d.Teardown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Teardown error = %v, want context.Canceled", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "canceled.csv")); !os.IsNotExist(err) {
		t.Fatalf("canceled teardown published merged output: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "MANIFEST.json")); !os.IsNotExist(err) {
		t.Fatalf("canceled teardown published manifest: %v", err)
	}

	shards, err := filepath.Glob(filepath.Join(workDir, ".shards", "canceled.w*.csv"))
	if err != nil {
		t.Fatalf("glob shards: %v", err)
	}

	if len(shards) != 1 {
		t.Fatalf("recoverable shards = %v, want one shard", shards)
	}

	assertNoTemporaryOutputs(t, workDir)

	if err := d.Teardown(context.Background()); err != nil {
		t.Fatalf("retry Teardown: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "canceled.csv")); err != nil {
		t.Fatalf("retry did not publish merged output: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "MANIFEST.json")); err != nil {
		t.Fatalf("retry did not publish manifest: %v", err)
	}
}

func TestAtomicOutputCancellationDuringCopy(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	outPath := filepath.Join(t.TempDir(), "merged.csv")
	payload := bytes.Repeat([]byte("x"), csvBufferSize*3)

	err := writeAtomic(ctx, outPath, func(out *os.File) error {
		dst := &cancelAfterWrite{dst: out, cancel: cancel}

		return copyContext(ctx, dst, bytes.NewReader(payload))
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("writeAtomic error = %v, want context.Canceled", err)
	}

	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("canceled copy published final output: %v", err)
	}

	assertNoTemporaryOutputs(t, filepath.Dir(outPath))
}

type cancelAfterWrite struct {
	dst    io.Writer
	cancel context.CancelFunc
}

func (w *cancelAfterWrite) Write(p []byte) (int, error) {
	written, err := w.dst.Write(p)
	w.cancel()

	return written, err
}

func assertNoTemporaryOutputs(t *testing.T, dir string) {
	t.Helper()

	var temps []string

	for _, pattern := range []string{
		filepath.Join(dir, ".*.tmp-*"),
		filepath.Join(dir, ".shards", ".*.tmp-*"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob temporary outputs: %v", err)
		}

		temps = append(temps, matches...)
	}

	if len(temps) != 0 {
		t.Fatalf("temporary outputs remain: %v", temps)
	}
}

// TestInsertRejectsNonNative verifies the typed CSV path rejects any
// method other than NATIVE.
func TestInsertRejectsNonNative(t *testing.T) {
	t.Parallel()

	d, _ := newTestDriver(t, map[string]string{"merge": "true"})

	for _, method := range []driver.InsertMethod{
		driver.InsertPlainQuery,
		driver.InsertPlainBulk,
		driver.InsertColumnar,
	} {
		_, err := d.Insert(context.Background(), &driver.InsertRequest{
			Table: "t", Method: method, Workers: 1, Source: typedRowsSource(1),
		})
		if err == nil {
			t.Fatalf("method %s did not error", method)
		}
	}
}

// keep stdcsv referenced for test helpers imported elsewhere.
var _ = stdcsv.NewReader
