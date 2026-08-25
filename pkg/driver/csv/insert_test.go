package csv

import (
	"bytes"
	"context"
	stdcsv "encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

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

func TestInsertRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()

	d, _ := newTestDriver(t, nil)
	req := typedRowsReq("unsupported", 1, 1)
	req.Method = driver.InsertPlainQuery

	_, err := d.Insert(context.Background(), req)
	if !errors.Is(err, driver.ErrInsertMethodNotSupported) {
		t.Fatalf("Insert error = %v, want ErrInsertMethodNotSupported", err)
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
