//nolint:revive // package path `pkg/driver/common` is fixed by the plan (§B8).
package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
)

// --- proto builders (mirror those in runtime/flat_test.go; kept local
//     so the common package has no test-time dep on runtime internals).

func lit(value any) *dgproto.Expr {
	switch typed := value.(type) {
	case int64:
		return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
			Value: &dgproto.Literal_Int64{Int64: typed},
		}}}
	case string:
		return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
			Value: &dgproto.Literal_String_{String_: typed},
		}}}
	case bool:
		return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
			Value: &dgproto.Literal_Bool{Bool: typed},
		}}}
	default:
		panic("lit: unsupported type")
	}
}

func rowIndex() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_GLOBAL,
	}}}
}

func col(name string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: name}}}
}

func binOp(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: op, A: a, B: b,
	}}}
}

func callExpr(name string, args ...*dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{
		Func: name, Args: args,
	}}}
}

func ifExpr(cond, thenExpr, elseExpr *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{
		Cond: cond, Then: thenExpr, Else_: elseExpr,
	}}}
}

func dictAt(key string, index *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_DictAt{DictAt: &dgproto.DictAt{
		DictKey: key, Index: index,
	}}}
}

func attr(name string, e *dgproto.Expr) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: e}
}

func attrWithNull(name string, e *dgproto.Expr, rate float32, salt uint64) *dgproto.Attr {
	return &dgproto.Attr{Name: name, Expr: e, Null: &dgproto.Null{Rate: rate, SeedSalt: salt}}
}

// mixedSpec builds an InsertSpec exercising the full range of stage-B
// primitives at every row: row_id via binop, a dict lookup, a stdlib
// call that consumes the row_id, an if-expression, a nullable string,
// and a two-level arithmetic chain.
func mixedSpec(size int64) *dgproto.InsertSpec {
	dicts := map[string]*dgproto.Dict{
		"regions": {
			Columns: []string{"name"},
			Rows: []*dgproto.DictRow{
				{Values: []string{"africa"}},
				{Values: []string{"america"}},
				{Values: []string{"asia"}},
				{Values: []string{"europe"}},
				{Values: []string{"middle east"}},
			},
		},
	}

	attrs := []*dgproto.Attr{
		attr("row_id", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
		attr("region", dictAt("regions", rowIndex())),
		attr("label", callExpr("std.format", lit("id-%05d"), col("row_id"))),
		attr("bucket", ifExpr(
			binOp(dgproto.BinOp_LT, rowIndex(), lit(int64(500))),
			lit("A"),
			lit("B"),
		)),
		attr("chain", binOp(
			dgproto.BinOp_ADD,
			binOp(dgproto.BinOp_MUL, col("row_id"), lit(int64(3))),
			lit(int64(7)),
		)),
		attrWithNull("optional", lit("present"), 0.25, 0xA5A5A5A5DEADBEEF),
	}

	return &dgproto.InsertSpec{
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "mixed", Size: size},
			Attrs:       attrs,
			ColumnOrder: []string{"row_id", "region", "label", "bucket", "chain", "optional"},
		},
		Dicts: dicts,
	}
}

func rowIndexKind(kind dgproto.RowIndex_Kind) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: kind,
	}}}
}

func fixedDegree(count int64) *dgproto.Degree {
	return &dgproto.Degree{Kind: &dgproto.Degree_Fixed{Fixed: &dgproto.DegreeFixed{
		Count: count,
	}}}
}

func relationshipSpec(outerSize, degree, sizeHint int64) *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Table:  "rel_t",
		Method: dgproto.InsertMethod_NATIVE,
		Source: &dgproto.RelSource{
			Population: &dgproto.Population{Name: "lineitem", Size: sizeHint},
			Attrs: []*dgproto.Attr{
				attr("order_idx", rowIndexKind(dgproto.RowIndex_ENTITY)),
				attr("line_idx", rowIndexKind(dgproto.RowIndex_LINE)),
				attr("global_idx", rowIndexKind(dgproto.RowIndex_GLOBAL)),
			},
			ColumnOrder: []string{"order_idx", "line_idx", "global_idx"},
			LookupPops: []*dgproto.LookupPop{{
				Population:  &dgproto.Population{Name: "orders", Size: outerSize},
				Attrs:       []*dgproto.Attr{attr("o_id", rowIndex())},
				ColumnOrder: []string{"o_id"},
			}},
			Relationships: []*dgproto.Relationship{{
				Name: "orders_lineitem",
				Sides: []*dgproto.Side{
					{Population: "orders", Degree: fixedDegree(1)},
					{Population: "lineitem", Degree: fixedDegree(degree)},
				},
			}},
			Iter: "orders_lineitem",
		},
	}
}

// collectAllRows uses RunParallel to drain every chunk into one []string
// slice. Rows are rendered with fmt.Sprint so the comparison is
// canonical. The caller is responsible for sorting, since chunks arrive
// in worker-completion order.
func collectAllRows(ctx context.Context, spec *dgproto.InsertSpec, workers int) ([]string, error) {
	chunks := SplitChunks(spec.GetSource().GetPopulation().GetSize(), workers)

	var (
		mu   sync.Mutex
		rows []string
	)

	err := RunParallel(ctx, spec, chunks, func(_ context.Context, chunk Chunk, rt *runtime.Runtime) error {
		local := make([]string, 0, chunk.Count)

		for range chunk.Count {
			row, err := rt.Next()
			if err != nil {
				return fmt.Errorf("row: %w", err)
			}

			local = append(local, fmt.Sprint(row))
		}

		mu.Lock()

		rows = append(rows, local...)
		mu.Unlock()

		return nil
	})
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func TestRunParallelDeterminismAcrossWorkers(t *testing.T) {
	t.Parallel()

	const size = int64(1000)

	spec := mixedSpec(size)
	ctx := context.Background()

	workerCounts := []int{1, 4, 16}
	results := make(map[int][]string, len(workerCounts))

	for _, workers := range workerCounts {
		rows, err := collectAllRows(ctx, spec, workers)
		if err != nil {
			t.Fatalf("workers=%d: %v", workers, err)
		}

		if int64(len(rows)) != size {
			t.Fatalf("workers=%d: got %d rows, want %d", workers, len(rows), size)
		}

		sort.Strings(rows)
		results[workers] = rows
	}

	baseline := results[1]
	for _, workers := range workerCounts[1:] {
		if !reflect.DeepEqual(baseline, results[workers]) {
			t.Fatalf("workers=%d produced a different multiset than workers=1", workers)
		}
	}
}

func TestRunParallelByWorkersUsesRuntimeTotalRows(t *testing.T) {
	t.Parallel()

	const (
		outerSize = int64(5)
		degree    = int64(3)
		sizeHint  = int64(999)
		wantRows  = outerSize * degree
	)

	var drained atomic.Int64

	gotRows, err := RunParallelByWorkers(
		context.Background(),
		relationshipSpec(outerSize, degree, sizeHint),
		4,
		func(_ context.Context, chunk Chunk, rt *runtime.Runtime) error {
			drained.Add(chunk.Count)

			for range chunk.Count {
				if _, rowErr := rt.Next(); rowErr != nil {
					return fmt.Errorf("row: %w", rowErr)
				}
			}

			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunParallelByWorkers: %v", err)
	}

	if gotRows != wantRows {
		t.Fatalf("total rows = %d, want %d", gotRows, wantRows)
	}

	if drained.Load() != wantRows {
		t.Fatalf("drained rows = %d, want %d", drained.Load(), wantRows)
	}
}

func TestSplitChunksCoversRange(t *testing.T) {
	t.Parallel()

	cases := []struct {
		total   int64
		workers int
	}{
		{total: 0, workers: 1},
		{total: 0, workers: 4},
		{total: 1, workers: 1},
		{total: 1, workers: 8},
		{total: 10, workers: 3},
		{total: 100, workers: 4},
		{total: 1000, workers: 16},
		{total: 1001, workers: 16},
		{total: 7, workers: 0},
	}

	for _, tc := range cases {
		chunks := SplitChunks(tc.total, tc.workers)
		if len(chunks) == 0 {
			t.Fatalf("total=%d workers=%d: empty chunks slice", tc.total, tc.workers)
		}

		var (
			sum      int64
			expected int64
		)

		for i, chunk := range chunks {
			if chunk.Index != i {
				t.Fatalf("total=%d workers=%d: chunk %d has Index=%d", tc.total, tc.workers, i, chunk.Index)
			}

			if chunk.Start != expected {
				t.Fatalf(
					"total=%d workers=%d: chunk %d Start=%d, want %d (gap or overlap)",
					tc.total, tc.workers, i, chunk.Start, expected,
				)
			}

			if chunk.Count < 0 {
				t.Fatalf("total=%d workers=%d: chunk %d negative Count=%d", tc.total, tc.workers, i, chunk.Count)
			}

			expected = chunk.Start + chunk.Count
			sum += chunk.Count
		}

		if sum != tc.total {
			t.Fatalf("total=%d workers=%d: sum of counts=%d", tc.total, tc.workers, sum)
		}
	}
}

func TestRunParallelPropagatesError(t *testing.T) {
	t.Parallel()

	spec := mixedSpec(200)
	chunks := SplitChunks(200, 4)
	sentinel := errors.New("chunk failure")

	var (
		siblingAborted atomic.Bool
		siblingRan     atomic.Int32
	)

	chunkFn := func(ctx context.Context, chunk Chunk, rt *runtime.Runtime) error {
		if chunk.Index == 1 {
			return sentinel
		}

		siblingRan.Add(1)

		for range chunk.Count {
			select {
			case <-ctx.Done():
				siblingAborted.Store(true)

				return ctx.Err()
			default:
			}

			if _, rowErr := rt.Next(); rowErr != nil && !errors.Is(rowErr, io.EOF) {
				return fmt.Errorf("row: %w", rowErr)
			}

			// Introduce a tiny delay so the failing worker has time to
			// cancel the group context before this one finishes.
			time.Sleep(50 * time.Microsecond)
		}

		return nil
	}

	err := RunParallel(context.Background(), spec, chunks, chunkFn)
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}

	if siblingRan.Load() == 0 {
		t.Fatalf("no sibling worker started; cannot assert cancellation")
	}

	if !siblingAborted.Load() {
		t.Fatalf("sibling workers did not observe ctx cancellation")
	}
}

func TestRunParallelContextCancel(t *testing.T) {
	t.Parallel()

	spec := mixedSpec(10000)
	chunks := SplitChunks(10000, 4)

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, len(chunks))

	var (
		observed  atomic.Int32
		startOnce sync.Once
	)

	done := make(chan error, 1)

	go func() {
		done <- RunParallel(ctx, spec, chunks, func(ctx context.Context, chunk Chunk, rt *runtime.Runtime) error {
			startOnce.Do(func() { close(started) })

			for range chunk.Count {
				select {
				case <-ctx.Done():
					observed.Add(1)

					return ctx.Err()
				default:
				}

				if _, rowErr := rt.Next(); rowErr != nil && !errors.Is(rowErr, io.EOF) {
					return fmt.Errorf("row: %w", rowErr)
				}

				// Throttle so the cancel has time to land mid-chunk.
				time.Sleep(10 * time.Microsecond)
			}

			return nil
		})
	}()

	// Wait for at least one worker to begin before canceling.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("no worker started")
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("RunParallel did not return after ctx cancel")
	}

	if observed.Load() == 0 {
		t.Fatalf("no worker observed the cancellation")
	}
}

func TestRunParallelRejectsNilInputs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chunks := []Chunk{{Index: 0, Start: 0, Count: 1}}
	noop := func(context.Context, Chunk, *runtime.Runtime) error { return nil }

	if err := RunParallel(ctx, nil, chunks, noop); !errors.Is(err, ErrNilSpec) {
		t.Fatalf("nil spec: want ErrNilSpec, got %v", err)
	}

	if err := RunParallel(ctx, mixedSpec(1), chunks, nil); !errors.Is(err, ErrNilChunkFn) {
		t.Fatalf("nil fn: want ErrNilChunkFn, got %v", err)
	}

	if err := RunParallel(ctx, mixedSpec(1), nil, noop); !errors.Is(err, ErrNoChunks) {
		t.Fatalf("nil chunks: want ErrNoChunks, got %v", err)
	}
}
