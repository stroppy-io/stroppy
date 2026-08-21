package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/pkg/gen"
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

// ErrColumnCountMismatch is returned by the columnar path when the server
// Describe reports a different column count than the source declared.
var ErrColumnCountMismatch = errors.New("postgres: describe column count mismatch")

// ErrUnregisteredColumnType is returned by the columnar path when a target
// column has a type OID that pgx's type map cannot name for the array cast.
var ErrUnregisteredColumnType = errors.New("postgres: unregistered column type OID")

// Insert runs a typed [driver.InsertRequest] through the postgres driver.
// Each worker prepares a [gen.Cursor] partition, adapts it to a
// source.RowSource, and drains it through the same runInsertChunk the
// legacy path uses. Generation is allocation-free after preparation;
// driver-side row materialization and COPY/bulk encoding may allocate.
func (d *Driver) Insert(
	ctx context.Context,
	req *driver.InsertRequest,
) (*stats.Query, error) {
	if err := driver.ValidateInsert(req); err != nil { //nolint:errcheck // sentinel wrapped below
		return nil, err
	}

	workers := req.Workers
	if workers < 1 {
		workers = 1
	}

	columns := req.Source.Schema().ColumnNames()

	start := time.Now()

	rows, err := common.RunParallelBatch(ctx, req.Source, workers, d.bulkSize,
		func(workerCtx context.Context, _ common.Chunk, cur gen.Cursor) error {
			src := common.NewBatchRowSource(cur, columns, len(columns))

			return d.runInsertChunk(workerCtx, req.Table, req.Method, src)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// runInsertChunk dispatches one partition's output into the database per
// the request's InsertMethod. src is drained to EOF.
func (d *Driver) runInsertChunk(
	ctx context.Context,
	table string,
	method driver.InsertMethod,
	src source.RowSource,
) error {
	switch method {
	case driver.InsertNative:
		return d.copyFromRuntime(ctx, table, src)
	case driver.InsertPlainBulk:
		return d.bulkInsertRuntime(ctx, table, src, d.bulkSize)
	case driver.InsertColumnar:
		return d.columnarInsertRuntime(ctx, table, src, d.bulkSize)
	case driver.InsertPlainQuery:
		// Per-row INSERT reuses the bulk path with batch_size=1 so both
		// arms share exactly one SQL-building codepath.
		return d.bulkInsertRuntime(ctx, table, src, 1)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedInsertMethod, method)
	}
}

// statementCtx derives the per-statement deadline for a single COPY/INSERT.
func (d *Driver) statementCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return sqldriver.StatementTimeout(ctx, d.queryTimeout)
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

	stmtCtx, cancel := d.statementCtx(ctx)
	defer cancel()

	start := time.Now()
	rowsCopied, err := d.pool.CopyFrom(stmtCtx, pgx.Identifier{table}, src.Columns(), copySrc)
	copySrc.progress.Flush()

	if err != nil {
		return fmt.Errorf("postgres: CopyFrom %q: %w", table, statementContextError(stmtCtx, err))
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
		if err := insertprogress.Canceled(ctx); err != nil {
			return err
		}

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

	stmtCtx, cancel := d.statementCtx(ctx)
	defer cancel()

	if _, err := d.pool.Exec(stmtCtx, query, args...); err != nil {
		return fmt.Errorf("postgres: bulk INSERT %q: %w", table, statementContextError(stmtCtx, err))
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

// columnarInsertRuntime inserts via a single array parameter per column,
// expanded back to rows server-side with unnest():
//
//	INSERT INTO t (c1,...,cn) SELECT * FROM unnest($1::t1[],...,$n::tn[])
//
// Bound-parameter count equals the column count regardless of batch size, so
// this path never hits Postgres' 65535 bind-parameter ceiling that the
// row-major VALUES path does on wide tables. Column SQL types are read from the
// server (a Describe of the target columns), not guessed from Go values, so the
// casts match the catalog exactly. src is drained to io.EOF.
func (d *Driver) columnarInsertRuntime(
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

	castTypes, err := d.describeColumnCastTypes(ctx, table, columns)
	if err != nil {
		return err
	}

	// The unnest statement text is the same for every batch (row count lives in
	// the array arguments, not the SQL), so it is built once per worker.
	query := buildColumnarInsert(table, columns, castTypes)

	// One reusable array buffer per column; Exec consumes them synchronously so
	// they are safe to reset and refill across batches.
	cols := make([][]any, len(columns))
	for i := range cols {
		cols[i] = make([]any, 0, batchSize)
	}

	// args[0] pins the query-exec mode; the column arrays follow as $1..$n.
	// Encoding a []any array needs the server-assigned parameter OIDs, which only
	// a describe-based mode reports. The pool default (QueryExecModeExec, or the
	// simple protocol) skips the parameter Describe and leaves pgx to infer OIDs
	// from the Go values — impossible for a bare []any — so this path forces a
	// describe-based mode regardless of the pool default. pgx filters the mode
	// out of the positional bind args.
	args := make([]any, len(columns)+1)
	args[0] = pgx.QueryExecModeCacheDescribe
	filled := 0

	generatedProgress := insertprogress.NewGeneratedRowCounter(ctx)
	defer generatedProgress.Flush()

	insertprogress.SetStage(ctx, insertprogress.StageRuntimeNext)

	for {
		if err := insertprogress.Canceled(ctx); err != nil {
			return err
		}

		row, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("postgres: source.Next: %w", err)
		}

		appendRowColumnar(cols, row)

		filled++

		generatedProgress.Add(1)

		if filled >= batchSize {
			generatedProgress.Flush()

			if err := d.execProgressColumnarBatch(ctx, table, query, cols, args, int64(filled)); err != nil {
				return err
			}

			resetColumns(cols)

			filled = 0
		}
	}

	if filled > 0 {
		generatedProgress.Flush()

		if err := d.execProgressColumnarBatch(ctx, table, query, cols, args, int64(filled)); err != nil {
			return err
		}
	}

	return nil
}

// appendRowColumnar scatters one row's cells into the per-column buffers. The
// source reuses its scratch row slice across Next calls; scalar cells are copied
// by value on append, but []byte cells alias the reused buffer and are cloned so
// they survive until the batch flushes.
func appendRowColumnar(cols [][]any, row []any) {
	for i, v := range row {
		if b, ok := v.([]byte); ok {
			cp := make([]byte, len(b))
			copy(cp, b)
			v = cp
		}

		cols[i] = append(cols[i], v)
	}
}

// resetColumns truncates every per-column buffer while keeping its backing
// array for the next batch.
func resetColumns(cols [][]any) {
	for i := range cols {
		cols[i] = cols[i][:0]
	}
}

func (d *Driver) execProgressColumnarBatch(
	ctx context.Context,
	table string,
	query string,
	cols [][]any,
	args []any,
	rows int64,
) error {
	insertprogress.SetStage(ctx, insertprogress.StagePostgresColumnarExec)

	start := time.Now()

	// args[0] is the pinned exec mode; column arrays bind to $1..$n after it.
	for i, col := range cols {
		args[i+1] = col
	}

	stmtCtx, cancel := d.statementCtx(ctx)
	defer cancel()

	if _, err := d.pool.Exec(stmtCtx, query, args...); err != nil {
		return fmt.Errorf("postgres: columnar INSERT %q: %w", table, statementContextError(stmtCtx, err))
	}

	insertprogress.AddConfirmed(ctx, rows)
	insertprogress.AddBatch(ctx, rows, time.Since(start))
	insertprogress.SetStage(ctx, insertprogress.StageRuntimeNext)

	return nil
}

// describeColumnCastTypes asks the server for the SQL type of each target
// column and returns the per-column array-cast type name (e.g. "int8", "text").
// It prepares a `SELECT <cols> FROM <table>` and reads the resulting field OIDs,
// mirroring how pgx.CopyFrom sources column types from the catalog rather than
// inferring them from Go values.
func (d *Driver) describeColumnCastTypes(
	ctx context.Context,
	table string,
	columns []string,
) ([]string, error) {
	stmtCtx, cancel := d.statementCtx(ctx)
	defer cancel()

	conn, err := d.pool.Acquire(stmtCtx)
	if err != nil {
		return nil, fmt.Errorf(
			"postgres: acquire conn for describe %q: %w",
			table,
			statementContextError(stmtCtx, err),
		)
	}
	defer conn.Release()

	var sb strings.Builder

	sb.WriteString("SELECT ")

	for i, col := range columns {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(pgx.Identifier{col}.Sanitize())
	}

	sb.WriteString(" FROM ")
	sb.WriteString(pgx.Identifier{table}.Sanitize())

	sd, err := conn.Conn().Prepare(stmtCtx, "", sb.String())
	if err != nil {
		return nil, fmt.Errorf(
			"postgres: describe columns of %q: %w",
			table,
			statementContextError(stmtCtx, err),
		)
	}

	if len(sd.Fields) != len(columns) {
		return nil, fmt.Errorf(
			"%w: table %q returned %d, want %d",
			ErrColumnCountMismatch, table, len(sd.Fields), len(columns),
		)
	}

	typeMap := conn.Conn().TypeMap()
	casts := make([]string, len(sd.Fields))

	for i, field := range sd.Fields {
		pgType, ok := typeMap.TypeForOID(field.DataTypeOID)
		if !ok {
			column := columns[i] //nolint:gosec // field and column counts are checked above

			return nil, fmt.Errorf(
				"%w: table %q column %q OID %d",
				ErrUnregisteredColumnType, table, column, field.DataTypeOID,
			)
		}

		casts[i] = pgType.Name
	}

	return casts, nil
}

// buildColumnarInsert returns the unnest-based INSERT statement. Each column
// gets one numbered placeholder cast to its array type; the parameter count is
// the column count, independent of how many rows a batch carries.
func buildColumnarInsert(table string, columns, castTypes []string) string {
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

	sb.WriteString(") SELECT * FROM unnest(")

	for i, cast := range castTypes {
		if i > 0 {
			sb.WriteString(", ")
		}

		fmt.Fprintf(&sb, "$%d::%s[]", i+1, cast)
	}

	sb.WriteString(")")

	return sb.String()
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
