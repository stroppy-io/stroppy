package simple

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// drainAll prepares a full-range cursor over req.Source and materializes
// every row into a [][]any in entity order.
func drainAll(t *testing.T, req *driver.InsertRequest) [][]any {
	t.Helper()

	src := req.Source
	cols := src.Schema().ColumnNames()

	cur, err := src.Prepare(0, -1, 64)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	var rows [][]any

	scratch := make([]any, len(cols))

	for {
		b, err := cur.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			t.Fatalf("Next: %v", err)
		}

		for i := range b.Len() {
			b.MaterializeRow(i, scratch)

			cp := make([]any, len(cols))
			copy(cp, scratch)
			rows = append(rows, cp)
		}
	}

	return rows
}

// TestDemoSourceCardinality verifies the demo source emits exactly
// demoRows rows with 1-based sequential ids.
func TestDemoSourceCardinality(t *testing.T) {
	t.Parallel()

	rows := drainAll(t, demoInsertRequest())

	if len(rows) != demoRows {
		t.Fatalf("rows = %d, want %d", len(rows), demoRows)
	}

	for i, row := range rows {
		id, ok := row[0].(int64)
		if !ok || id != int64(i+1) {
			t.Fatalf("row %d id = %v, want %d", i, row[0], i+1)
		}
	}
}

// TestDemoSourceLabelShape verifies every label is exactly 8 characters
// drawn from [A-Za-z].
func TestDemoSourceLabelShape(t *testing.T) {
	t.Parallel()

	rows := drainAll(t, demoInsertRequest())

	for i, row := range rows {
		label, ok := row[1].(string)
		if !ok {
			t.Fatalf("row %d label type %T", i, row[1])
		}

		if len(label) != 8 {
			t.Fatalf("row %d label len = %d, want 8", i, len(label))
		}

		for _, c := range label {
			isAlpha := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
			if !isAlpha {
				t.Fatalf("row %d label %q has non-alpha byte", i, label)
			}
		}
	}
}

// TestDemoSourceValueRange verifies every value lands in [0, 999].
func TestDemoSourceValueRange(t *testing.T) {
	t.Parallel()

	rows := drainAll(t, demoInsertRequest())

	for i, row := range rows {
		v, ok := row[2].(int64)
		if !ok {
			t.Fatalf("row %d value type %T", i, row[2])
		}

		if v < 0 || v > 999 {
			t.Fatalf("row %d value = %d, want [0,999]", i, v)
		}
	}
}

// TestDemoSourceWorkerInvariance verifies the source produces the same
// id set regardless of worker count (drained through the typed parallel
// runner). The id set must be exactly {1..demoRows}.
func TestDemoSourceWorkerInvariance(t *testing.T) {
	t.Parallel()

	req := demoInsertRequest()
	cols := req.Source.Schema().ColumnNames()

	for workers := 1; workers <= 4; workers *= 2 {
		var (
			seen []int64
			mu   sync.Mutex
		)

		_, err := common.RunParallelBatch(context.Background(), req.Source, workers, 16,
			func(_ context.Context, _ common.Chunk, cur gen.Cursor) error {
				src := common.NewBatchRowSource(cur, cols, len(cols))

				var local []int64

				for {
					row, err := src.Next()
					if err != nil {
						if errors.Is(err, io.EOF) {
							break
						}

						return err
					}

					local = append(local, row[0].(int64))
				}

				mu.Lock()

				seen = append(seen, local...)
				mu.Unlock()

				return nil
			})
		if err != nil {
			t.Fatalf("workers=%d: %v", workers, err)
		}

		if len(seen) != demoRows {
			t.Fatalf("workers=%d: rows %d, want %d", workers, len(seen), demoRows)
		}

		got := make(map[int64]struct{}, demoRows)
		for _, id := range seen {
			got[id] = struct{}{}
		}

		if len(got) != demoRows {
			t.Fatalf("workers=%d: distinct ids %d, want %d", workers, len(got), demoRows)
		}

		for id := int64(1); id <= demoRows; id++ {
			if _, ok := got[id]; !ok {
				t.Fatalf("workers=%d: missing id %d", workers, id)
			}
		}
	}
}

// TestDemoSourceSteadyZeroAlloc verifies the demo row formula allocates
// nothing on steady-state batch fills after preparation.
func TestDemoSourceSteadyZeroAlloc(t *testing.T) {
	src := demoSource(gen.New(demoSeed), 1<<20, 8)

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

// TestDemoSourceFirstFillZeroAlloc verifies the first fill on a freshly
// prepared cursor allocates nothing. Cursors are pre-prepared (the batch
// allocation is preparation, not generation) so the measured call covers
// only the row formula.
func TestDemoSourceFirstFillZeroAlloc(t *testing.T) {
	src := demoSource(gen.New(demoSeed), 1<<40, 8)

	cursors := make([]gen.Cursor, 200)
	for i := range cursors {
		cur, err := src.Prepare(int64(i)*8, 8, 8)
		if err != nil {
			t.Fatalf("prepare %d: %v", i, err)
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
