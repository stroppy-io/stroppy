package csv

import (
	"context"
	stdcsv "encoding/csv"
	"errors"
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
	b := gen.NewSchemaBuilder()
	idCol := b.Int64("id")
	sqCol := b.Int64("squared")
	labelCol := b.Bytes("label", 3)
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
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

	// .shards/ must be cleaned up by the merge pass.
	if _, err := os.Stat(filepath.Join(workDir, ".shards")); !os.IsNotExist(err) {
		t.Fatalf(".shards dir still present after merge: %v", err)
	}
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
