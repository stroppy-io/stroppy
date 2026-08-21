package bench

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/tpcdsgen"
	"github.com/stroppy-io/stroppy/pkg/datagen/tpchgen"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
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

// QueryValue returns the first column of the first row (or nil if no rows).
func (b *Bench) QueryValue(ctx context.Context, sql string, args map[string]any) (_ any, err error) {
	res, err := b.runQuery(ctx, sql, args)
	defer func() { err = b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	return firstQueryValue(res.Rows)
}

func firstQueryValue(rows driver.Rows) (any, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}

		//nolint:nilnil // no-row sentinel: workloads branch on `v == nil` after `err == nil`
		return nil, nil
	}

	vals := rows.Values()
	if len(vals) == 0 {
		//nolint:nilnil // no-row sentinel: empty row means "no value"; same caller contract as above
		return nil, nil
	}

	return vals[0], rows.Err()
}

// QueryRow returns the first row (or nil if no rows).
func (b *Bench) QueryRow(ctx context.Context, sql string, args map[string]any) (_ []any, err error) {
	res, err := b.runQuery(ctx, sql, args)
	defer func() { err = b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	if !res.Rows.Next() {
		return nil, res.Rows.Err()
	}

	return res.Rows.Values(), res.Rows.Err()
}

// QueryRows returns all rows (up to a large cap).
func (b *Bench) QueryRows(ctx context.Context, sql string, args map[string]any) (_ [][]any, err error) {
	res, err := b.runQuery(ctx, sql, args)
	defer func() { err = b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	return res.Rows.ReadAll(0), res.Rows.Err()
}

func (b *Bench) runQuery(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	res, err := b.drv.RunQuery(ctx, sql, args)
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

// Insert runs a typed [driver.InsertRequest] through the benchmark driver,
// the typed successor to InsertSpec. It wires the progress tracker and
// metrics recording, streaming rows from a workload-authored
// [gen.BatchSource] instead of a dgproto generator.
func (b *Bench) Insert(ctx context.Context, req *driver.InsertRequest) (*stats.Query, error) {
	if err := driver.ValidateInsert(req); err != nil {
		return nil, fmt.Errorf("insert: %w", err)
	}

	tracker := b.newBatchInsertTracker(req)

	runCtx := ctx
	if tracker.Enabled() {
		runCtx = insertprogress.ContextWithTracker(ctx, tracker)
		tracker.Start(runCtx)
	}

	result, err := b.drv.Insert(runCtx, req)
	if tracker.Enabled() {
		tracker.Finish(err)
	}

	var elapsed time.Duration
	if result != nil {
		elapsed = result.Elapsed
	}

	b.root.txMetrics.recordInsertResult(b.vu, req.Table, elapsed, err)

	if err != nil {
		return nil, fmt.Errorf("insert %q: %w", req.Table, err)
	}

	b.root.txMetrics.recordInsert(b.vu, req.Table, result.Rows)

	return result, nil
}

// newBatchInsertTracker builds the progress tracker for the typed Insert
// path from the request's table/method/workers, mirroring the legacy
// spec-driven tracker.
func (b *Bench) newBatchInsertTracker(req *driver.InsertRequest) *insertprogress.Tracker {
	config := insertprogress.DefaultConfig()
	config.Table = req.Table
	config.Method = req.Method.String()
	config.Workers = req.Workers
	config.Logger = b.lg.Named("insert-progress")
	config.OnSample = func(snapshot insertprogress.Snapshot) {
		b.root.txMetrics.recordInsertProgress(b.vu, &snapshot)
	}

	return insertprogress.NewTracker(&config)
}

// InsertTpch loads one TPC-H table via the ported dbgen generator, streamed
// through the typed [driver.InsertRequest] path. The dbgen generator lives
// behind a [gen.BatchSource] adapter (tpchgen.NewBatchSource), so no dgproto
// InsertSpec is synthesized; canonical seeds, seeking, and entity fan-out are
// preserved unchanged.
func (b *Bench) InsertTpch(ctx context.Context, table string, scaleFactor float64, workers int) (*stats.Query, error) {
	if workers < 1 {
		workers = 1
	}

	src, err := tpchgen.NewBatchSource(table, scaleFactor)
	if err != nil {
		return nil, fmt.Errorf("tpch %q: %w", table, err)
	}

	req := &driver.InsertRequest{
		Table: table, Method: driver.InsertNative, Workers: workers, Source: src,
	}

	return b.Insert(ctx, req)
}

// InsertTpcds loads one TPC-DS table via the ported dsdgen generator, streamed
// through the typed [driver.InsertRequest] path. The dsdgen generator lives
// behind a [gen.BatchSource] adapter (tpcdsgen.NewBatchSource), so no dgproto
// InsertSpec is synthesized; canonical text, null semantics, ticket fan-out,
// and per-partition seeking are preserved unchanged.
func (b *Bench) InsertTpcds(ctx context.Context, table string, scaleFactor float64, workers int) (*stats.Query, error) {
	if workers < 1 {
		workers = 1
	}

	src, err := tpcdsgen.NewBatchSource(table, scaleFactor)
	if err != nil {
		return nil, fmt.Errorf("tpcds %q: %w", table, err)
	}

	req := &driver.InsertRequest{
		Table: table, Method: driver.InsertNative, Workers: workers, Source: src,
	}

	return b.Insert(ctx, req)
}

// Begin starts a transaction.
func (b *Bench) Begin(ctx context.Context, opts BeginOpts) (*TxX, error) {
	iso, err := ParseTxIsolation(string(opts.Isolation))
	if err != nil {
		return nil, err
	}

	if iso == stroppy.TxIsolationLevel_NONE {
		return &TxX{tx: nil, b: b, iso: iso, name: opts.Name, start: time.Now()}, nil
	}

	tx, err := b.drv.Begin(ctx, iso)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}

	return &TxX{tx: tx, b: b, iso: iso, name: opts.Name, start: time.Now()}, nil
}

// BeginTx runs fn inside a transaction: commits on nil return, rolls back on error.
func (b *Bench) BeginTx(ctx context.Context, opts BeginOpts, fn func(*TxX) error) error {
	tx, err := b.Begin(ctx, opts)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)

		return err
	}

	return tx.Commit(ctx)
}

// TxX is the transaction handle (sugar over driver.Tx; NONE mode delegates to the driver).
type TxX struct {
	tx      driver.Tx
	b       *Bench
	iso     stroppy.TxIsolationLevel
	name    string
	start   time.Time
	queries int
	done    bool
}

func (t *TxX) Exec(ctx context.Context, sql string, args map[string]any) error {
	res, err := t.runQuery(ctx, sql, args)

	return t.b.finishQuery(res, err)
}

func (t *TxX) QueryValue(ctx context.Context, sql string, args map[string]any) (_ any, err error) {
	res, err := t.runQuery(ctx, sql, args)
	defer func() { err = t.b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	return firstQueryValue(res.Rows)
}

func (t *TxX) QueryRow(ctx context.Context, sql string, args map[string]any) (_ []any, err error) {
	res, err := t.runQuery(ctx, sql, args)
	defer func() { err = t.b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	if !res.Rows.Next() {
		return nil, res.Rows.Err()
	}

	return res.Rows.Values(), res.Rows.Err()
}

func (t *TxX) QueryRows(ctx context.Context, sql string, args map[string]any) (_ [][]any, err error) {
	res, err := t.runQuery(ctx, sql, args)
	defer func() { err = t.b.finishQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	return res.Rows.ReadAll(0), res.Rows.Err()
}

func (t *TxX) runQuery(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	t.queries++
	if t.tx == nil {
		// NONE mode: delegate to the parent driver.
		res, err := t.b.drv.RunQuery(ctx, sql, args)
		if err != nil {
			return nil, fmt.Errorf("query: %w", err)
		}

		return res, nil
	}

	res, err := t.tx.RunQuery(ctx, sql, args)
	if err != nil {
		return nil, fmt.Errorf("tx query: %w", err)
	}

	return res, nil
}

func (t *TxX) Commit(ctx context.Context) error {
	committed := true

	if t.tx != nil {
		if err := t.tx.Commit(ctx); err != nil {
			return err
		}
	}

	t.b.root.txMetrics.record(t.b.vu, "commit", t.name, t.iso)
	t.recordEnd("commit", committed)

	return nil
}

func (t *TxX) Rollback(ctx context.Context) error {
	if t.tx != nil {
		if err := t.tx.Rollback(ctx); err != nil {
			return err
		}
	}

	t.b.root.txMetrics.record(t.b.vu, "rollback", t.name, t.iso)
	t.recordEnd("rollback", false)

	return nil
}

// recordEnd emits the per-transaction summary metrics (total duration, commit
// rate, query count) once. Idempotent — safe if a workload calls both paths.
func (t *TxX) recordEnd(action string, committed bool) {
	if t.done {
		return
	}

	t.done = true
	t.b.root.txMetrics.recordTxEnd(
		t.b.vu, action, t.name, t.iso, time.Since(t.start), t.queries, committed,
	)
}

// Compile-tick: ensure zap import stays used if expanded later.
var _ = zap.NewNop
