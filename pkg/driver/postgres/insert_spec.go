package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// ErrUnsupportedInsertMethod is returned when an InsertSpec requests a
// method the postgres driver cannot serve. Today every arm of
// dgproto.InsertMethod is supported, but new enum values land here before
// the switch learns them.
var ErrUnsupportedInsertMethod = errors.New("postgres: unsupported InsertSpec method")

// ErrEmptyColumnOrder is returned by the bulk insert path when the
// runtime reports zero columns; a multi-row INSERT would be degenerate
// without them.
var ErrEmptyColumnOrder = errors.New("postgres: runtime reports zero columns")

// InsertSpec runs one relational InsertSpec through the postgres driver.
// It builds a seed runtime.Runtime from the spec, then dispatches by
// spec.Method to one of three row-insertion strategies (NATIVE COPY,
// PLAIN_BULK multi-row INSERT, PLAIN_QUERY per-row INSERT). When the
// spec requests parallelism the seed runtime is cloned per worker via
// common.RunParallel; each clone is pre-seeked to its chunk boundary.
func (d *Driver) InsertSpec(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	if spec == nil {
		return nil, fmt.Errorf("%w: nil spec", runtime.ErrInvalidSpec)
	}

	workers := int(spec.GetParallelism().GetWorkers())
	if workers <= 1 {
		return d.insertSpecSingle(ctx, spec)
	}

	return d.insertSpecParallel(ctx, spec, workers)
}

// insertSpecSingle runs the spec on a single seed Runtime without the
// overhead of RunParallel when the caller requested workers ≤ 1.
func (d *Driver) insertSpecSingle(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		return nil, fmt.Errorf("postgres: build runtime: %w", err)
	}

	start := time.Now()

	if err := d.runChunk(ctx, spec, rt, -1); err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

// insertSpecParallel fans the spec out across workers goroutines via
// common.RunParallel. Each worker owns an independent Runtime clone
// pre-seeked to its chunk.Start; per-worker row counts are accumulated
// atomically and reported back on the final stats.Query.
func (d *Driver) insertSpecParallel(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	workers int,
) (*stats.Query, error) {
	total := spec.GetSource().GetPopulation().GetSize()
	chunks := common.SplitChunks(total, workers)

	start := time.Now()

	err := common.RunParallel(ctx, spec, chunks,
		func(workerCtx context.Context, chunk common.Chunk, rt *runtime.Runtime) error {
			return d.runChunk(workerCtx, spec, rt, chunk.Count)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

// runChunk dispatches one runtime's output into the database per the
// spec's InsertMethod. When count is negative the runtime is drained to
// EOF; otherwise it emits exactly count rows before stopping.
func (d *Driver) runChunk(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	rt *runtime.Runtime,
	count int64,
) error {
	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE:
		return d.copyFromRuntime(ctx, spec.GetTable(), rt, count)
	case dgproto.InsertMethod_PLAIN_BULK:
		return d.bulkInsertRuntime(ctx, spec.GetTable(), rt, count, d.bulkSize)
	case dgproto.InsertMethod_PLAIN_QUERY:
		// Per-row INSERT reuses the bulk path with batch_size=1 so both
		// arms share exactly one SQL-building codepath.
		return d.bulkInsertRuntime(ctx, spec.GetTable(), rt, count, 1)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedInsertMethod, spec.GetMethod().String())
	}
}

// copyFromRuntime streams runtime rows into pgx.CopyFrom without buffering
// the full result set. The adapter bounds emission by `limit`, or drains
// to EOF when limit < 0.
func (d *Driver) copyFromRuntime(
	ctx context.Context,
	table string,
	rt *runtime.Runtime,
	limit int64,
) error {
	src := &rowSource{rt: rt, limit: limit}

	if _, err := d.pool.CopyFrom(ctx, pgx.Identifier{table}, rt.Columns(), src); err != nil {
		return fmt.Errorf("postgres: CopyFrom %q: %w", table, err)
	}

	return nil
}

// bulkInsertRuntime emits multi-row INSERT statements of up to batchSize
// rows each. It exhausts the runtime (or stops after `limit` rows when
// limit ≥ 0). Placeholders are pgx's numbered $1,$2,... form.
func (d *Driver) bulkInsertRuntime(
	ctx context.Context,
	table string,
	rt *runtime.Runtime,
	limit int64,
	batchSize int,
) error {
	if batchSize < 1 {
		batchSize = 1
	}

	columns := rt.Columns()
	if len(columns) == 0 {
		return fmt.Errorf("%w: table %q", ErrEmptyColumnOrder, table)
	}

	batch := make([][]any, 0, batchSize)
	remaining := limit

	for limit < 0 || remaining > 0 {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("postgres: runtime.Next: %w", err)
		}

		// Copy the row: Runtime reuses its scratch slice across calls.
		rowCopy := make([]any, len(row))
		copy(rowCopy, row)
		batch = append(batch, rowCopy)

		if limit >= 0 {
			remaining--
		}

		if len(batch) >= batchSize {
			if err := d.execBulkBatch(ctx, table, columns, batch); err != nil {
				return err
			}

			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		if err := d.execBulkBatch(ctx, table, columns, batch); err != nil {
			return err
		}
	}

	return nil
}

// execBulkBatch assembles and executes a multi-row INSERT for the given
// rows. Placeholders are numbered left-to-right; arguments are appended
// in row-major order.
func (d *Driver) execBulkBatch(
	ctx context.Context,
	table string,
	columns []string,
	rows [][]any,
) error {
	query, args := buildBulkInsert(table, columns, rows)

	if _, err := d.pool.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("postgres: bulk INSERT %q: %w", table, err)
	}

	return nil
}

// buildBulkInsert returns a multi-row INSERT statement for the given
// table and rows, plus the flattened argument list. Identifiers are
// quoted with pgx.Identifier so reserved words survive.
func buildBulkInsert(table string, columns []string, rows [][]any) (query string, args []any) {
	var sb strings.Builder

	sb.WriteString("INSERT INTO ")
	sb.WriteString(pgx.Identifier{table}.Sanitize())
	sb.WriteString(" (")

	for i, col := range columns {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(pgx.Identifier{col}.Sanitize())
	}

	sb.WriteString(") VALUES ")

	args = make([]any, 0, len(rows)*len(columns))
	placeholder := 1

	for rowIdx, row := range rows {
		if rowIdx > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString("(")

		for colIdx := range row {
			if colIdx > 0 {
				sb.WriteString(", ")
			}

			fmt.Fprintf(&sb, "$%d", placeholder)
			placeholder++
		}

		sb.WriteString(")")

		args = append(args, row...)
	}

	query = sb.String()

	return query, args
}

// rowSource adapts *runtime.Runtime to pgx.CopyFromSource. Each Next()
// call pulls one row from the runtime; emission stops at EOF or after
// `limit` rows when limit ≥ 0. Errors are stored and surfaced via Err().
type rowSource struct {
	rt    *runtime.Runtime
	limit int64 // < 0 means unbounded
	row   []any
	err   error
	sent  int64
}

// Next advances the runtime cursor. Returns false at EOF, on error, or
// when the configured limit has been reached.
func (s *rowSource) Next() bool {
	if s.err != nil {
		return false
	}

	if s.limit >= 0 && s.sent >= s.limit {
		return false
	}

	row, err := s.rt.Next()
	if errors.Is(err, io.EOF) {
		return false
	}

	if err != nil {
		s.err = err

		return false
	}

	s.row = row
	s.sent++

	return true
}

// Values returns the current row. pgx calls Values once per successful
// Next, so the runtime's scratch slice is safe to return directly —
// pgx.CopyFrom serializes each row before advancing.
func (s *rowSource) Values() ([]any, error) { return s.row, nil }

// Err reports any runtime error encountered during iteration. pgx
// aborts the COPY transaction when Err is non-nil.
func (s *rowSource) Err() error { return s.err }
