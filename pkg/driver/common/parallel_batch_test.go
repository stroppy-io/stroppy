package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/gen"
)

// simpleSchema is a 2-column schema (id int64, name bytes) reused by
// the typed-path tests. It returns the schema and the bound column
// handles a RowFunc needs.
func simpleSchema() (gen.Schema, gen.Column, gen.Column) {
	b := gen.NewSchemaBuilder()
	idCol := b.Int64("id")
	nameCol := b.Bytes("name", 8)

	return b.Build(), idCol, nameCol
}

// simpleSource returns an indexed source over simpleSchema whose rows
// are (entity, a deterministic length-N string). totalRows is the
// entity count.
func simpleSource(totalRows int64) *gen.IndexedSource {
	schema, idCol, nameCol := simpleSchema()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(idCol, int64(entity))

		n := int(entity % 8)

		dst, err := r.Bytes(nameCol, n)
		if err != nil {
			return err
		}

		for i := range n {
			dst[i] = byte('a' + (int(entity)+i)%26)
		}

		return nil
	}

	return gen.NewIndexedSource(
		schema, gen.Root{}, "test/simple@1", totalRows, 64, fn,
	)
}

// TestRunParallelBatchRowCount verifies the typed runner drains every
// row exactly once across workers.
func TestRunParallelBatchRowCount(t *testing.T) {
	t.Parallel()

	const total = int64(10000)

	for _, workers := range []int{1, 2, 4, 7} {
		var seen int64

		_, err := RunParallelBatch(context.Background(), simpleSource(total), workers, 64,
			func(_ context.Context, _ Chunk, cur gen.Cursor) error {
				for {
					b, err := cur.Next()
					if err != nil {
						if errors.Is(err, io.EOF) {
							return nil
						}

						return err
					}

					atomic.AddInt64(&seen, int64(b.Len()))
				}
			})
		if err != nil {
			t.Fatalf("workers=%d: %v", workers, err)
		}

		if seen != total {
			t.Fatalf("workers=%d: saw %d, want %d", workers, seen, total)
		}
	}
}

// TestRunParallelBatchWorkerInvariance verifies the typed runner produces
// a byte-identical row stream regardless of worker count (the core
// worker-invariance contract inherited from the indexed source).
func TestRunParallelBatchWorkerInvariance(t *testing.T) {
	t.Parallel()

	const total = int64(2000)

	schema, _, _ := simpleSchema()
	cols := schema.ColumnNames()

	readAll := func(workers int) []string {
		var (
			rows []string
			mu   sync.Mutex
		)

		_, err := RunParallelBatch(context.Background(), simpleSource(total), workers, 32,
			func(_ context.Context, _ Chunk, cur gen.Cursor) error {
				src := NewBatchRowSource(cur, cols, len(cols))

				var local []string

				for {
					row, err := src.Next()
					if err != nil {
						if errors.Is(err, io.EOF) {
							break
						}

						return err
					}

					local = append(local, fmt.Sprintf("%d/%s", row[0].(int64), row[1].(string)))
				}

				mu.Lock()

				rows = append(rows, local...)
				mu.Unlock()

				return nil
			})
		if err != nil {
			t.Fatalf("workers=%d: %v", workers, err)
		}

		return rows
	}

	// Worker 0 produces entities in order 0..total-1 only when single-
	// worker; with multiple workers the per-worker segments are contiguous
	// but interleaved. So invariance here is on the SET of rows, not order:
	// collect into a counted set and compare across worker counts.
	count := func(rows []string) map[string]int {
		m := make(map[string]int, len(rows))
		for _, r := range rows {
			m[r]++
		}

		return m
	}

	base := count(readAll(1))
	if len(base) != int(total) {
		t.Fatalf("single-worker row count = %d, want %d", len(base), total)
	}

	for _, workers := range []int{2, 4, 8} {
		got := count(readAll(workers))
		if len(got) != len(base) {
			t.Fatalf("workers=%d: distinct rows %d, want %d", workers, len(got), len(base))
		}

		for r, n := range base {
			if got[r] != n {
				t.Fatalf("workers=%d: row %q count %d, want %d", workers, r, got[r], n)
			}
		}
	}
}

// TestRunParallelBatchNilGuards verifies the typed runner rejects a nil
// source and nil fn.
func TestRunParallelBatchNilGuards(t *testing.T) {
	t.Parallel()

	if _, err := RunParallelBatch(context.Background(), nil, 1, 64,
		func(context.Context, Chunk, gen.Cursor) error { return nil }); err == nil {
		t.Fatal("nil source did not error")
	}

	if _, err := RunParallelBatch(context.Background(), simpleSource(1), 1, 64, nil); err == nil {
		t.Fatal("nil fn did not error")
	}
}

// TestBatchRowSourceAdaptsCursor verifies the adapter materializes every
// row of a prepared cursor in order and signals io.EOF at the end.
func TestBatchRowSourceAdaptsCursor(t *testing.T) {
	t.Parallel()

	src := simpleSource(100)
	schema := src.Schema()
	cols := schema.ColumnNames()

	cur, err := src.Prepare(0, -1, 16)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	ad := NewBatchRowSource(cur, cols, len(cols))

	var (
		last  int64 = -1
		count int64
	)

	for {
		row, err := ad.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			t.Fatalf("Next: %v", err)
		}

		id := row[0].(int64)
		if id != last+1 {
			t.Fatalf("id = %d, want %d (ordered)", id, last+1)
		}

		last = id
		count++
	}

	if count != 100 {
		t.Fatalf("count = %d, want 100", count)
	}
}
