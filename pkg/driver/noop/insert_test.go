package noop

import (
	"context"
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// noopSource returns an indexed source over (id int64) whose rows are
// the entity index. total is the entity count.
func noopSource(total int64) *gen.IndexedSource {
	b := gen.NewSchemaBuilder()
	idCol := b.Int64("id")
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(idCol, int64(entity))

		return nil
	}

	return gen.NewIndexedSource(schema, gen.Root{}, "test/noop@1", total, 64, fn)
}

func noopReq(method driver.InsertMethod, total int64, workers int) *driver.InsertRequest {
	return &driver.InsertRequest{
		Table:   "t",
		Method:  method,
		Workers: workers,
		Source:  noopSource(total),
	}
}

// newNoopDriver builds a noop driver with a valid options struct.
func newNoopDriver(t *testing.T) *Driver {
	t.Helper()

	return NewDriver(driver.Options{Config: &config.DriverConfig{}})
}

// TestInsertNilGuards verifies Insert rejects a nil request, nil source,
// and zero-column schema before dispatching workers.
func TestInsertNilGuards(t *testing.T) {
	t.Parallel()

	d := newNoopDriver(t)

	if _, err := d.Insert(context.Background(), nil); err == nil {
		t.Fatal("nil request did not error")
	}

	if _, err := d.Insert(context.Background(), &driver.InsertRequest{
		Table: "t", Method: driver.InsertNative, Workers: 1,
	}); err == nil {
		t.Fatal("nil source did not error")
	}

	empty := gen.NewIndexedSource(
		gen.NewSchemaBuilder().Build(), gen.Root{}, "test/empty@1", 0, 1,
		func(gen.Row, uint64) error { return nil },
	)
	if _, err := d.Insert(context.Background(), &driver.InsertRequest{
		Table: "t", Method: driver.InsertNative, Workers: 1, Source: empty,
	}); err == nil {
		t.Fatal("empty schema did not error")
	}
}

// TestInsertRowCountPerMethod verifies every advertised insert method
// drains the same row count through the noop driver.
func TestInsertRowCountPerMethod(t *testing.T) {
	t.Parallel()

	const total = int64(5000)

	for _, method := range []driver.InsertMethod{
		driver.InsertPlainQuery,
		driver.InsertPlainBulk,
		driver.InsertColumnar,
		driver.InsertNative,
	} {
		d := newNoopDriver(t)

		res, err := d.Insert(context.Background(), noopReq(method, total, 4))
		if err != nil {
			t.Fatalf("method %s: %v", method, err)
		}

		if res.Rows != total {
			t.Fatalf("method %s: rows %d, want %d", method, res.Rows, total)
		}
	}
}

// TestInsertWorkerInvariance verifies the noop typed path reports the
// same row count regardless of worker count (the core invariance
// contract inherited from the indexed source).
func TestInsertWorkerInvariance(t *testing.T) {
	t.Parallel()

	const total = int64(10000)

	for _, workers := range []int{1, 2, 4, 8, 16} {
		d := newNoopDriver(t)

		res, err := d.Insert(context.Background(), noopReq(driver.InsertNative, total, workers))
		if err != nil {
			t.Fatalf("workers=%d: %v", workers, err)
		}

		if res.Rows != total {
			t.Fatalf("workers=%d: rows %d, want %d", workers, res.Rows, total)
		}
	}
}

// TestInsertEmptyPopulation verifies a zero-row source drains cleanly
// (no error, zero rows).
func TestInsertEmptyPopulation(t *testing.T) {
	t.Parallel()

	d := newNoopDriver(t)

	res, err := d.Insert(context.Background(), noopReq(driver.InsertNative, 0, 4))
	if err != nil {
		t.Fatalf("empty population: %v", err)
	}

	if res.Rows != 0 {
		t.Fatalf("empty population rows %d, want 0", res.Rows)
	}
}

// TestInsertContextCancel verifies the noop path honors context
// cancellation.
func TestInsertContextCancel(t *testing.T) {
	t.Parallel()

	d := newNoopDriver(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.Insert(ctx, noopReq(driver.InsertNative, 100, 1))
	if err == nil {
		t.Fatal("canceled context did not error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
