package bench

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/v6/internal/rowmap"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
)

// BeginOpts selects isolation + names the tx for metrics.
type BeginOpts struct {
	Isolation TxIsolationName
	Name      string
}

// Exec runs a statement that returns no rows.
func (b *Bench) Exec(ctx context.Context, sql string, args map[string]any) error {
	res, err := b.runQuery(ctx, sql, args)

	return b.finishQuery(res, err)
}

// QueryValue reads one owned scalar or shallow struct. No row is ErrNoRows.
func (b *Bench) QueryValue[T any](ctx context.Context, sql string, args map[string]any) (T, error) {
	row, err := b.queryRow(ctx, sql, args)
	if err != nil {
		var zero T

		return zero, err
	}

	return rowmap.Decode[T](row.columns, row.values)
}

// QueryValues collects owned scalar or shallow struct rows and closes the cursor.
func (b *Bench) QueryValues[T any](ctx context.Context, sql string, args map[string]any) (values []T, err error) {
	rows, err := b.Query(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	defer func() { err = driver.JoinErrors(err, rows.Close()) }()

	values = make([]T, 0)

	for rows.Next() {
		value, decodeErr := rowmap.Decode[T](rows.columns, rows.source.Values())
		if decodeErr != nil {
			return nil, decodeErr
		}

		values = append(values, value)
	}

	return values, rows.Err()
}

func firstQueryRow(rows driver.Rows) ([]any, error) {
	if rows == nil {
		return nil, nil
	}

	if !rows.Next() {
		return nil, rows.Err()
	}

	return copyValues(rows.Values()), rows.Err()
}

func readQueryRows(rows driver.Rows) ([][]any, error) {
	if rows == nil {
		return nil, nil
	}

	out := [][]any{}
	for rows.Next() {
		out = append(out, copyValues(rows.Values()))
	}

	return out, rows.Err()
}

// QueryRow returns the first row (or nil if no rows).
func (b *Bench) RawRow(ctx context.Context, sql string, args map[string]any) (_ []any, err error) {
	res, err := b.runQuery(ctx, sql, args)
	defer func() { err = b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	return firstQueryRow(res.Rows)
}

// QueryRows returns all rows (up to a large cap).
func (b *Bench) RawRows(ctx context.Context, sql string, args map[string]any) (_ [][]any, err error) {
	res, err := b.runQuery(ctx, sql, args)
	defer func() { err = b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	return readQueryRows(res.Rows)
}

func (b *Bench) runQuery(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	if err := b.ensureDriver(ctx); err != nil {
		return nil, err
	}

	res, err := b.drv.RunQuery(b.operationContext(ctx), sql, args)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	return res, nil
}

func (b *Bench) finishQuery(res *driver.QueryResult, queryErr error) error {
	if res != nil && res.Rows != nil {
		closeErr := res.Rows.Close()
		queryErr = driver.JoinErrors(queryErr, res.Rows.Err(), closeErr)
	}

	var elapsed time.Duration
	if res != nil && res.Stats != nil {
		elapsed = res.Stats.Elapsed
	}

	b.root.txMetrics.recordQueryResult(b.vu, elapsed, queryErr)

	return queryErr
}

// Insert runs a [driver.InsertRequest] through the benchmark driver. It wires
// progress tracking and metrics while streaming workload-authored
// [gen.BatchSource] rows.
// InsertResult contains neutral insertion measurements.
type InsertResult struct {
	Rows    int64
	Elapsed time.Duration
}
type InsertStrategy int

const (
	InsertPlainQuery InsertStrategy = InsertStrategy(driver.InsertPlainQuery)
	InsertPlainBulk  InsertStrategy = InsertStrategy(driver.InsertPlainBulk)
	InsertColumnar   InsertStrategy = InsertStrategy(driver.InsertColumnar)
	InsertNative     InsertStrategy = InsertStrategy(driver.InsertNative)
)

func (m InsertStrategy) String() string { return driver.InsertMethod(m).String() }

type (
	InsertOption interface {
		applyInsert(request *driver.InsertRequest)
	}
	insertOption func(*driver.InsertRequest)
)

func (o insertOption) applyInsert(r *driver.InsertRequest) { o(r) }
func InsertMethod(m InsertStrategy) InsertOption {
	return insertOption(func(r *driver.InsertRequest) { r.Method = driver.InsertMethod(m) })
}

func LoadWorkers(n int) InsertOption {
	if n < 1 {
		invalid("load workers", inputError("must be positive"))
	}

	return insertOption(func(r *driver.InsertRequest) { r.Workers = n })
}

// Insert streams a partitioned source through the selected database backend.
func (b *Bench) Insert(
	ctx context.Context,
	table string,
	source gen.BatchSource,
	options ...InsertOption,
) (*InsertResult, error) {
	req := &driver.InsertRequest{Table: table, Source: source, Workers: 1}

	for _, option := range options {
		if option == nil {
			invalid("insert", inputError("nil option"))
		}

		option.applyInsert(req)
	}

	result, err := b.insert(ctx, req)
	if result == nil {
		return nil, err
	}

	return &InsertResult{Rows: result.Rows, Elapsed: result.Elapsed}, err
}

func (b *Bench) insert(ctx context.Context, req *driver.InsertRequest) (*stats.Query, error) {
	if err := driver.ValidateInsert(req); err != nil {
		return nil, fmt.Errorf("insert: %w", err)
	}

	if err := b.ensureDriver(ctx); err != nil {
		return nil, err
	}

	effectiveReq := *req
	if effectiveReq.Method == 0 && b.cfg.GetDefaultInsertMethod() != "" {
		method, err := driver.ResolveInsertMethod(b.cfg.DriverType, b.cfg.GetDefaultInsertMethod())
		if err != nil {
			return nil, fmt.Errorf("insert: %w", err)
		}

		effectiveReq.Method = method
	}

	if !driver.SupportsInsertMethod(b.cfg.DriverType, effectiveReq.Method) {
		return nil, fmt.Errorf("insert: %w %q (%s driver)",
			driver.ErrInsertMethodUnsupported, effectiveReq.Method.String(), b.cfg.DriverType)
	}

	tracker := b.newBatchInsertTracker(&effectiveReq)

	runCtx := ctx
	if tracker.Enabled() {
		runCtx = insertprogress.ContextWithTracker(ctx, tracker)
		tracker.Start(runCtx)

		defer func() {
			if value := recover(); value != nil {
				tracker.Finish(inputError("insert action panicked"))
				panic(value)
			}
		}()
	}

	result, err := b.drv.Insert(b.operationContext(runCtx), &effectiveReq)
	if tracker.Enabled() {
		tracker.Finish(err)
	}

	var elapsed time.Duration
	if result != nil {
		elapsed = result.Elapsed
	}

	b.root.txMetrics.recordInsertResult(b.vu, effectiveReq.Table, elapsed, err)

	if err != nil {
		return nil, fmt.Errorf("insert %q: %w", effectiveReq.Table, err)
	}

	b.root.txMetrics.recordInsert(b.vu, effectiveReq.Table, result.Rows)

	return result, nil
}

// newBatchInsertTracker builds the progress tracker from the request's
// table, method, and worker count.
func (b *Bench) newBatchInsertTracker(req *driver.InsertRequest) *insertprogress.Tracker {
	cfg := insertprogress.DefaultConfig()
	cfg.Table = req.Table
	cfg.Method = req.Method.String()
	cfg.Workers = req.Workers
	cfg.Logger = b.lg.Named("insert-progress")
	cfg.OnSample = func(snapshot insertprogress.Snapshot) {
		b.root.txMetrics.recordInsertProgress(b.vu, &snapshot)
	}

	return insertprogress.NewTracker(&cfg)
}

// Begin starts a transaction.
func (b *Bench) Begin(ctx context.Context, opts BeginOpts) (*Tx, error) {
	if err := b.ensureDriver(ctx); err != nil {
		return nil, err
	}

	iso, err := ParseTxIsolation(string(opts.Isolation))
	if err != nil {
		return nil, err
	}

	if iso == config.TxIsolationLevelNone {
		return &Tx{tx: nil, b: b, iso: iso, name: opts.Name, start: time.Now()}, nil
	}

	tx, err := b.drv.Begin(b.operationContext(ctx), iso)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}

	return &Tx{tx: tx, b: b, iso: iso, name: opts.Name, start: time.Now()}, nil
}

// BeginTx runs fn inside a transaction: commits on nil return, rolls back on error.
func (b *Bench) BeginTx(ctx context.Context, opts BeginOpts, fn func(*Tx) error) (err error) {
	tx, err := b.Begin(ctx, opts)
	if err != nil {
		return err
	}
	defer func() {
		if !tx.done {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultCleanupTimeout)
			defer cancel()

			err = driver.JoinErrors(err, tx.Rollback(cleanup))
		}
	}()

	if bodyErr := fn(tx); bodyErr != nil {
		return bodyErr
	}

	return tx.Commit(ctx)
}

// Tx is the transaction handle (sugar over driver.Tx; NONE mode delegates to the driver).
type Tx struct {
	tx      driver.Tx
	b       *Bench
	iso     config.TxIsolationLevel
	name    string
	start   time.Time
	queries int
	done    bool
}

func (t *Tx) Exec(ctx context.Context, sql string, args map[string]any) error {
	res, err := t.runQuery(ctx, sql, args)

	return t.b.finishQuery(res, err)
}

func (t *Tx) QueryValue[T any](ctx context.Context, sql string, args map[string]any) (T, error) {
	row, err := t.queryRow(ctx, sql, args)
	if err != nil {
		var zero T

		return zero, err
	}

	return rowmap.Decode[T](row.columns, row.values)
}

func (t *Tx) QueryValues[T any](ctx context.Context, sql string, args map[string]any) (values []T, err error) {
	rows, err := t.Query(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	defer func() { err = driver.JoinErrors(err, rows.Close()) }()

	values = make([]T, 0)

	for rows.Next() {
		v, decodeErr := rowmap.Decode[T](rows.columns, rows.source.Values())
		if decodeErr != nil {
			return nil, decodeErr
		}

		values = append(values, v)
	}

	return values, rows.Err()
}

func (t *Tx) RawRow(ctx context.Context, sql string, args map[string]any) (_ []any, err error) {
	res, err := t.runQuery(ctx, sql, args)
	defer func() { err = t.b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	return firstQueryRow(res.Rows)
}

func (t *Tx) RawRows(ctx context.Context, sql string, args map[string]any) (_ [][]any, err error) {
	res, err := t.runQuery(ctx, sql, args)
	defer func() { err = t.b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	return readQueryRows(res.Rows)
}

func (t *Tx) runQuery(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	if t.done {
		return nil, errTransactionClosed
	}

	t.queries++
	if t.tx == nil {
		// NONE mode: delegate to the parent driver.
		res, err := t.b.drv.RunQuery(t.b.operationContext(ctx), sql, args)
		if err != nil {
			return nil, fmt.Errorf("query: %w", err)
		}

		return res, nil
	}

	res, err := t.tx.RunQuery(t.b.operationContext(ctx), sql, args)
	if err != nil {
		return nil, fmt.Errorf("tx query: %w", err)
	}

	return res, nil
}

func (t *Tx) Commit(ctx context.Context) error {
	if t.done {
		return errTransactionClosed
	}

	committed := true

	if t.tx != nil {
		if err := t.tx.Commit(t.b.operationContext(ctx)); err != nil {
			return err
		}
	}

	t.b.root.txMetrics.record(t.b.vu, "commit", t.name, t.iso)
	t.recordEnd("commit", committed)

	return nil
}

var errTransactionClosed = errors.New("transaction already finalized")

func (t *Tx) Rollback(ctx context.Context) error {
	if t.done {
		return errTransactionClosed
	}

	if t.tx != nil {
		if err := t.tx.Rollback(t.b.operationContext(ctx)); err != nil {
			return err
		}
	}

	t.b.root.txMetrics.record(t.b.vu, "rollback", t.name, t.iso)
	t.recordEnd("rollback", false)

	return nil
}

// recordEnd emits the per-transaction summary metrics (total duration, commit
// rate, query count) once. Idempotent — safe if a workload calls both paths.
func (t *Tx) recordEnd(action string, committed bool) {
	if t.done {
		return
	}

	t.done = true
	t.b.root.txMetrics.recordTxEnd(
		t.b.vu, action, t.name, t.iso, time.Since(t.start), t.queries, committed,
	)
}
