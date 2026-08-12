package gen_test

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/pkg/gen"
)

// exampleItemSchema builds the schema used across batch tests and returns the
// bound column handles.
func exampleItemSchema() (gen.Schema, idCols) {
	b := gen.NewSchemaBuilder()
	c := idCols{
		id:    b.Int64("i_id"),
		imID:  b.Int64("i_im_id"),
		name:  b.Bytes("i_name", 24),
		price: b.Float64("i_price"),
		flag:  b.Bool("i_flag"),
		since: b.Time("i_since"),
	}

	return b.Build(), c
}

type idCols struct {
	id    gen.Column
	imID  gen.Column
	name  gen.Column
	price gen.Column
	flag  gen.Column
	since gen.Column
}

// itemFields holds the per-field deterministic streams for item generation.
type itemFields struct {
	name  gen.Field
	imID  gen.Field
	price gen.Field
}

func itemFieldsOf(root gen.Root) itemFields {
	d := root.Domain("tpcc/item@1")

	return itemFields{
		name:  d.Field("i_name"),
		imID:  d.Field("i_im_id"),
		price: d.Field("i_price"),
	}
}

// itemRow generates one item row at entity into r. The name is a fixed 20-byte
// fill from the i_name field; the length is fixed here to keep the batch-path
// example focused on typed storage rather than length drawing.
func itemRow(r gen.Row, entity uint64, c idCols, f itemFields) error {
	r.SetInt64(c.id, int64(entity+1))
	r.SetInt64(c.imID, f.imID.Int64(entity, 1, 10000))

	dst, err := r.Bytes(c.name, 20)
	if err != nil {
		return err
	}

	d := f.name.At(entity)
	gen.AlphaNumeric.Fill(&d, dst)

	r.SetFloat64(c.price, f.price.Decimal(entity, 1, 100, 2))
	r.SetBool(c.flag, entity%2 == 0)
	r.SetTime(c.since, time.Unix(int64(entity)*86400, 0).UTC())

	return nil
}

func TestSchemaRejectsDuplicateColumn(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("duplicate column did not panic")
		}
	}()

	b := gen.NewSchemaBuilder()
	b.Int64("x")
	b.Int64("x")
}

func TestSchemaColumnNames(t *testing.T) {
	t.Parallel()

	s, _ := exampleItemSchema()
	want := []string{"i_id", "i_im_id", "i_name", "i_price", "i_flag", "i_since"}

	got := s.ColumnNames()
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}

	for i, n := range got {
		if n != want[i] {
			t.Fatalf("col %d: got %q want %q", i, n, want[i])
		}
	}
}

type readRow struct {
	id    int64
	imID  int64
	name  []byte
	price float64
	flag  bool
	since time.Time
}

func readBatch(b *gen.Batch, c idCols) []readRow {
	out := make([]readRow, 0, b.Len())
	for i := range b.Len() {
		r := b.Row(i)
		out = append(out, readRow{
			id:    r.Int64Val(c.id),
			imID:  r.Int64Val(c.imID),
			name:  append([]byte(nil), r.BytesVal(c.name)...),
			price: r.Float64Val(c.price),
			flag:  r.BoolVal(c.flag),
			since: r.TimeVal(c.since),
		})
	}

	return out
}

func drain(t *testing.T, root gen.Root, domain string, start, count int64, c idCols, f itemFields) []readRow {
	t.Helper()

	schema, _ := exampleItemSchema()
	src := gen.NewIndexedSource(schema, root, domain, start+count, 4, func(r gen.Row, entity uint64) error {
		return itemRow(r, entity, c, f)
	})

	cur, err := src.Prepare(start, count, 4)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	var rows []readRow

	for {
		b, err := cur.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("next: %v", err)
		}

		rows = append(rows, readBatch(b, c)...)
	}

	return rows
}

func TestBatchFillAndRead(t *testing.T) {
	t.Parallel()

	schema, c := exampleItemSchema()
	root := gen.New(0xC0FFEE)
	f := itemFieldsOf(root)
	src := gen.NewIndexedSource(schema, root, "tpcc/item@1", 7, 4, func(r gen.Row, entity uint64) error {
		return itemRow(r, entity, c, f)
	})

	cur, err := src.Prepare(0, -1, 4)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	rows := drainFromCursor(t, cur, c)
	if len(rows) != 7 {
		t.Fatalf("got %d rows, want 7", len(rows))
	}

	for i, r := range rows {
		if r.id != int64(i+1) {
			t.Fatalf("row %d id %d", i, r.id)
		}

		if len(r.name) != 20 {
			t.Fatalf("row %d name len %d", i, len(r.name))
		}
	}
}

func drainFromCursor(t *testing.T, cur gen.Cursor, c idCols) []readRow {
	t.Helper()

	var rows []readRow

	for {
		b, err := cur.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("next: %v", err)
		}

		rows = append(rows, readBatch(b, c)...)
	}

	return rows
}

func TestBatchSeekContinuation(t *testing.T) {
	t.Parallel()

	root := gen.New(31)
	f := itemFieldsOf(root)

	const N = 100

	full := drain(t, root, "seek/item@1", 0, N, idColsOf(), f)

	suffix := drain(t, root, "seek/item@1", 50, 50, idColsOf(), f)
	if len(suffix) != 50 {
		t.Fatalf("suffix len %d", len(suffix))
	}

	for i := range suffix {
		if suffix[i].id != full[50+i].id || suffix[i].imID != full[50+i].imID || !sameBytes(suffix[i].name, full[50+i].name) {
			t.Fatalf("suffix %d differs from full", i)
		}
	}
}

func idColsOf() idCols {
	_, c := exampleItemSchema()

	return c
}

func sameBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestWorkerInvarianceBatch(t *testing.T) {
	t.Parallel()

	root := gen.New(41)
	f := itemFieldsOf(root)

	const N = 1000

	single := drain(t, root, "inv/item@1", 0, N, idColsOf(), f)

	for _, workers := range []int{1, 2, 4, 8} {
		chunk := (N + workers - 1) / workers
		results := make([][]readRow, workers)

		var wg sync.WaitGroup

		for w := range workers {
			start := w * chunk
			if start >= N {
				continue
			}

			end := start + chunk
			if end > N {
				end = N
			}

			wg.Add(1)
			go func(idx, s, e int) {
				defer wg.Done()

				results[idx] = drain(t, root, "inv/item@1", int64(s), int64(e-s), idColsOf(), f)
			}(w, start, end)
		}

		wg.Wait()

		pos := 0

		for w := range workers {
			for _, r := range results[w] {
				if r.id != single[pos].id || r.imID != single[pos].imID || !sameBytes(r.name, single[pos].name) {
					t.Fatalf("workers=%d entity %d differs", workers, pos)
				}

				pos++
			}
		}

		if pos != N {
			t.Fatalf("workers=%d total %d, want %d", workers, pos, N)
		}
	}
}

func TestBytesOverflowRejected(t *testing.T) {
	t.Parallel()

	b := gen.NewSchemaBuilder()
	col := b.Bytes("s", 8)
	schema := b.Build()
	batch := gen.NewBatch(schema, 4)

	r := batch.Row(0)
	if _, err := r.Bytes(col, 8); err != nil {
		t.Fatalf("length==budget should succeed: %v", err)
	}

	if _, err := r.Bytes(col, 9); err == nil {
		t.Fatalf("length>budget should error")
	}
}

func TestBatchResetClearsNulls(t *testing.T) {
	t.Parallel()

	b := gen.NewSchemaBuilder()
	col := b.Int64("v")
	schema := b.Build()
	batch := gen.NewBatch(schema, 2)
	r0 := batch.Row(0)
	r0.SetInt64(col, 5)
	r0.SetNull(col)

	if !r0.IsNull(col) {
		t.Fatalf("null not set")
	}

	batch.Reset()
	r0 = batch.Row(0)
	r0.SetInt64(col, 6)

	if r0.IsNull(col) {
		t.Fatalf("reset did not clear null")
	}
}

// TestAllocsSteadyFill verifies steady-state generation after preparation
// allocates nothing.
func TestAllocsSteadyFill(t *testing.T) {
	schema, c := exampleItemSchema()
	root := gen.New(55)
	f := itemFieldsOf(root)
	src := gen.NewIndexedSource(schema, root, "alloc/item@1", 1<<20, 8, func(r gen.Row, entity uint64) error {
		return itemRow(r, entity, c, f)
	})

	cur, err := src.Prepare(0, -1, 8)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	if _, err := cur.Next(); err != nil { // warm steady-state
		t.Fatalf("warm: %v", err)
	}

	if n := testing.AllocsPerRun(100, func() {
		if _, err := cur.Next(); err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("next: %v", err)
		}
	}); n != 0 {
		t.Fatalf("steady fill allocs = %v, want 0", n)
	}
}

// TestAllocsFirstFill verifies the first fill on a freshly prepared cursor
// allocates nothing — no lazy allocation hidden behind the first Next.
func TestAllocsFirstFill(t *testing.T) {
	schema, c := exampleItemSchema()
	root := gen.New(56)
	f := itemFieldsOf(root)

	cursors := make([]gen.Cursor, 200)
	for i := range cursors {
		src := gen.NewIndexedSource(schema, root, "alloc/item@1", 1<<40, 8, func(r gen.Row, entity uint64) error {
			return itemRow(r, entity, c, f)
		})

		cur, err := src.Prepare(int64(i)*8, 8, 8)
		if err != nil {
			t.Fatalf("prepare: %v", err)
		}

		cursors[i] = cur
	}

	var i int

	if n := testing.AllocsPerRun(100, func() {
		if _, err := cursors[i].Next(); err != nil {
			t.Fatalf("first fill: %v", err)
		}

		i++
	}); n != 0 {
		t.Fatalf("first fill allocs = %v, want 0", n)
	}
}

func TestConcurrentCursors(t *testing.T) {
	t.Parallel()

	schema, c := exampleItemSchema()
	root := gen.New(77)
	f := itemFieldsOf(root)

	const workers, perWorker = 8, 64

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			src := gen.NewIndexedSource(schema, root, "race/item@1", workers*perWorker, 8, func(r gen.Row, entity uint64) error {
				return itemRow(r, entity, c, f)
			})

			cur, err := src.Prepare(int64(idx)*perWorker, perWorker, 8)
			if err != nil {
				t.Errorf("prepare: %v", err)

				return
			}

			var n int

			for {
				b, err := cur.Next()
				if errors.Is(err, io.EOF) {
					break
				}

				if err != nil {
					t.Errorf("next: %v", err)

					return
				}

				n += b.Len()
			}

			if n != perWorker {
				t.Errorf("worker %d rows %d, want %d", idx, n, perWorker)
			}
		}(w)
	}

	wg.Wait()
}

func TestEmptySourceEOF(t *testing.T) {
	t.Parallel()

	schema, _ := exampleItemSchema()
	src := gen.NewIndexedSource(schema, gen.New(0), "empty@1", 0, 4, func(r gen.Row, entity uint64) error {
		return nil
	})

	cur, err := src.Prepare(0, -1, 4)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	b, err := cur.Next()
	if !errors.Is(err, io.EOF) || b != nil {
		t.Fatalf("got %v %v, want EOF nil", b, err)
	}
}
