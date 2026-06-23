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
	"github.com/stroppy-io/stroppy/pkg/datagen/loadsource"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// ErrUnsupportedInsertMethod is returned when an InsertSpec requests a
// method the postgres driver cannot serve. Today every arm of
// dgproto.InsertMethod is supported, but new enum values land here before
// the switch learns them.
var ErrUnsupportedInsertMethod = errors.New("postgres: unsupported InsertSpec method")

// ErrEmptyColumnOrder is returned by the bulk insert path when the
// source reports zero columns; a multi-row INSERT would be degenerate
// without them.
var ErrEmptyColumnOrder = errors.New("postgres: source reports zero columns")

// InsertSpec runs one relational InsertSpec through the postgres driver.
// It builds a source.Partitionable from the spec, then dispatches by
// spec.Method to one of three row-insertion strategies (NATIVE COPY,
// PLAIN_BULK multi-row INSERT, PLAIN_QUERY per-row INSERT). Workers fan
// the spec out across per-partition RowSources via
// common.RunParallelByWorkers; each RowSource is pre-seeked and bounded
// to its chunk.
func (d *Driver) InsertSpec(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	if spec == nil {
		return nil, fmt.Errorf("%w: nil spec", runtime.ErrInvalidSpec)
	}

	p, err := loadsource.Build(spec)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}

	workers := int(spec.GetParallelism().GetWorkers())
	if workers < 1 {
		workers = 1
	}

	start := time.Now()

	rows, err := common.RunParallelByWorkers(ctx, p, workers,
		func(workerCtx context.Context, _ common.Chunk, src source.RowSource) error {
			return d.runChunk(workerCtx, spec, src)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// runChunk dispatches one partition's output into the database per the
// spec's InsertMethod. src is drained to EOF.
func (d *Driver) runChunk(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	src source.RowSource,
) error {
	switch spec.GetMethod() {
	case dgproto.InsertMethod_NATIVE:
		return d.copyFromRuntime(ctx, spec.GetTable(), src)
	case dgproto.InsertMethod_PLAIN_BULK:
		return d.bulkInsertRuntime(ctx, spec.GetTable(), src, d.bulkSize)
	case dgproto.InsertMethod_PLAIN_QUERY:
		// Per-row INSERT reuses the bulk path with batch_size=1 so both
		// arms share exactly one SQL-building codepath.
		return d.bulkInsertRuntime(ctx, spec.GetTable(), src, 1)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedInsertMethod, spec.GetMethod().String())
	}
}

// copyFromRuntime streams source rows into pgx.CopyFrom without buffering
// the full result set. The adapter drains src to EOF.
func (d *Driver) copyFromRuntime(
	ctx context.Context,
	table string,
	src source.RowSource,
) error {
	insertprogress.SetStage(ctx, insertprogress.StagePostgresCopyFrom)
	copySrc := &rowSource{
		src:      src,
		progress: insertprogress.NewGeneratedRowCounter(ctx),
	}

	start := time.Now()
	rowsCopied, err := d.pool.CopyFrom(ctx, pgx.Identifier{table}, src.Columns(), copySrc)
	copySrc.progress.Flush()

	if err != nil {
		return fmt.Errorf("postgres: CopyFrom %q: %w", table, err)
	}

	insertprogress.AddConfirmed(ctx, rowsCopied)
	insertprogress.AddBatch(ctx, rowsCopied, time.Since(start))
	insertprogress.SetStage(ctx, insertprogress.StageRuntimeNext)

	return nil
}

// bulkInsertRuntime emits multi-row INSERT statements of up to batchSize
// rows each, draining src to io.EOF. Placeholders are pgx's numbered
// $1,$2,... form.
func (d *Driver) bulkInsertRuntime(
	ctx context.Context,
	table string,
	src source.RowSource,
	batchSize int,
) error {
	if batchSize < 1 {
		batchSize = 1
	}

	columns := src.Columns()
	if len(columns) == 0 {
		return fmt.Errorf("%w: table %q", ErrEmptyColumnOrder, table)
	}

	batch := make([][]any, 0, batchSize)

	generatedProgress := insertprogress.NewGeneratedRowCounter(ctx)
	defer generatedProgress.Flush()

	insertprogress.SetStage(ctx, insertprogress.StageRuntimeNext)

	for {
		row, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("postgres: source.Next: %w", err)
		}

		// Copy the row: the source reuses its scratch slice across calls.
		rowCopy := make([]any, len(row))
		copy(rowCopy, row)
		batch = append(batch, rowCopy)

		generatedProgress.Add(1)

		if len(batch) >= batchSize {
			generatedProgress.Flush()

			if err := d.execProgressBulkBatch(ctx, table, columns, batch); err != nil {
				return err
			}

			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		generatedProgress.Flush()

		if err := d.execProgressBulkBatch(ctx, table, columns, batch); err != nil {
			return err
		}
	}

	return nil
}

func (d *Driver) execProgressBulkBatch(
	ctx context.Context,
	table string,
	columns []string,
	batch [][]any,
) error {
	rows := int64(len(batch))

	insertprogress.SetStage(ctx, insertprogress.StagePostgresBulkInsertExec)

	start := time.Now()

	if err := d.execBulkBatch(ctx, table, columns, batch); err != nil {
		return err
	}

	insertprogress.AddConfirmed(ctx, rows)
	insertprogress.AddBatch(ctx, rows, time.Since(start))
	insertprogress.SetStage(ctx, insertprogress.StageRuntimeNext)

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

// rowSource adapts a source.RowSource to pgx.CopyFromSource. Each Next()
// call pulls one row from src; emission stops at EOF. Errors are stored
// and surfaced via Err().
type rowSource struct {
	src      source.RowSource
	progress insertprogress.RowCounter
	row      []any
	err      error
}

// Next advances the source cursor. Returns false at EOF or on error.
func (s *rowSource) Next() bool {
	if s.err != nil {
		return false
	}

	row, err := s.src.Next()
	if errors.Is(err, io.EOF) {
		return false
	}

	if err != nil {
		s.err = err

		return false
	}

	s.row = row
	s.progress.Add(1)

	return true
}

// Values returns the current row. pgx calls Values once per successful
// Next, so the source's scratch slice is safe to return directly —
// pgx.CopyFrom serializes each row before advancing.
func (s *rowSource) Values() ([]any, error) { return s.row, nil }

// Err reports any source error encountered during iteration. pgx
// aborts the COPY transaction when Err is non-nil.
func (s *rowSource) Err() error { return s.err }
