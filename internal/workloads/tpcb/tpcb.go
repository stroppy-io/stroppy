// Package tpcb is the Go-native port of pgbench's canonical 5-statement TPC-B
// transaction, shipping two registered variants:
//
//   - tpcb/tx runs the five DML steps inline under one client-side transaction
//     per iteration; supports pg/mysql/picodata/ydb.
//   - tpcb/procs calls the stored procedure tpcb_transaction (workload_procs
//     section) as a single server-side round-trip; pg/mysql only.
//
// Both variants share parameter declarations, schema/load lifecycle, retry
// policy, metrics, and data generation.
package tpcb

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

var (
	errAccountNotFound        = errors.New("tpc-b: account not found")
	errProcsDriverUnsupported = errors.New("tpcb/procs only supports postgres and mysql; use tpcb/tx for picodata/ydb")
	errProcsMissingQuery      = errors.New("tpc-b/procs: missing query")
)

// requiredTxQueries are the named transaction queries the measured iteration
// depends on. Every custom SQL file must provide them; a missing one would
// otherwise parse as an empty statement and run as a silent noop.
var requiredTxQueries = []struct{ section, query string }{
	{"workload_tx_tpcb", "update_account"},
	{"workload_tx_tpcb", "get_balance"},
	{"workload_tx_tpcb", "update_teller"},
	{"workload_tx_tpcb", "update_branch"},
	{"workload_tx_tpcb", "insert_history"},
}

// requiredSetupSections are the schema sections the setup steps execute. They are
// present in every dialect (unlike the pg/mysql-only index/fk/analyze sections,
// which legitimately no-op on picodata/ydb).
var requiredSetupSections = []string{"drop_schema", "create_schema"}

var (
	errMissingSection = errors.New("tpc-b: missing section")
	errMissingQuery   = errors.New("tpc-b: missing query")
	errEmptyQuery     = errors.New("tpc-b: empty query")
)

const (
	preset = "tpcb"

	branchesFiller = 88
	tellersFiller  = 84
	accountFiller  = 84

	tellersPerBranch  = 10
	accountsPerBranch = 100_000

	seedBranches = 0x7B01B
	seedTellers  = 0x7E11E
	seedAccounts = 0xACC07
)

type workload struct {
	variant string // "tx" (inline DML) or "procs" (stored procedure)

	sql           *bench.SQL
	procQuery     string // resolved workload_procs/tpcb_transaction (procs variant)
	driverType    bench.DriverTypeName
	iso           bench.TxIsolationName
	scale         int64
	retryAttempts int
	sqlFile       string
	loadWorkers   int

	retryMetricOnce sync.Once
	retryMetric     *bench.Metric
	retryPolicy     bench.RetryPolicy

	vuStates sync.Map // uint64 -> *vuState
}

func init() {
	bench.Register(func() bench.Workload { return &workload{variant: "tx"} })
	bench.Register(func() bench.Workload { return &workload{variant: "procs"} })
}

func (w *workload) Name() string { return "tpcb/" + w.variant }

func (w *workload) Define(d *bench.Def) error {
	w.scale = int64(max(d.Param.Int("scale-factor", 1, "TPC-B scale factor.").Value(), 1))
	w.retryAttempts = d.Param.Int("retry-attempts", 3, "Maximum transaction attempts.").Value()
	w.iso = bench.TxIsolationName(d.Param.String("tx-isolation", "", "Transaction isolation override.").Value())
	w.sqlFile = d.Param.String("sql-file", "", "SQL dialect file override.").Value()
	w.loadWorkers = d.Param.Int("load-workers", 1, "Workers used to load each table.").Value()
	w.loadWorkers = max(w.loadWorkers, 1)

	return nil
}

func (w *workload) Setup(ctx context.Context, b *bench.Bench) error {
	w.driverType = b.DriverTypeName()
	if w.variant == "procs" && !procsSupported(w.driverType) {
		return errProcsDriverUnsupported
	}

	w.iso = resolveIsolation(w.driverType, w.iso)
	w.sql = mustLoadSQL(w.driverType, w.sqlFile)

	if err := w.resolveSQLQueries(); err != nil {
		return err
	}

	runSection := func(name string) error {
		for _, q := range w.sql.Section(name) {
			if err := b.Exec(ctx, q, nil); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}

		return nil
	}

	type step struct {
		name string
		fn   func() error
	}

	steps := []step{
		{"drop_schema", func() error { return runSection("drop_schema") }},
		{"create_schema", func() error { return runSection("create_schema") }},
	}
	if w.variant == "procs" {
		steps = append(steps, step{"create_procedures", func() error { return runSection("create_procedures") }})
	}

	steps = append(steps,
		step{"load_data", func() error {
			if _, err := b.Insert(ctx, branchesRequest(w.scale, w.loadWorkers)); err != nil {
				return err
			}

			if _, err := b.Insert(ctx, tellersRequest(w.scale, w.loadWorkers)); err != nil {
				return err
			}

			if _, err := b.Insert(ctx, accountsRequest(w.scale, w.loadWorkers)); err != nil {
				return err
			}

			return nil
		}},
		step{"create_indexes", func() error { return runSection("create_indexes") }},
		step{"create_foreign_keys", func() error { return runSection("create_foreign_keys") }},
		step{"analyze", func() error { return runSection("analyze") }},
	)

	for _, s := range steps {
		if err := b.Step(s.name, s.fn); err != nil {
			return err
		}
	}

	b.StepBegin("workload")
	w.retryPolicy = b.TxRetryPolicy(bench.TxRetryPolicyOptions{
		MaxAttempts: w.retryAttempts,
		OnRetry:     func(int, error, bench.RetryDecision) { w.retryCounter(b).Add(1) },
	})

	return nil
}

func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	vs := w.vuState(b.VUID())
	aid, tid, bid, delta, hid := vs.txParams(w.scale)

	if w.variant == "procs" {
		return w.iterateProcs(ctx, b, aid, tid, bid, delta, hid)
	}

	return b.Step("workload", func() error {
		return bench.Retry0(ctx, w.retryPolicy, func() error {
			return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "tpcb"}, func(tx *bench.TxX) error {
				return w.txBody(ctx, tx, aid, tid, bid, delta, hid)
			})
		})
	})
}

// txBody runs the ordered DML of one client-side TPC-B transaction: update
// account, read balance, update teller, update branch, insert history. Extracted
// from Iterate to keep the transaction body out of the retry/begin closures.
func (w *workload) txBody(ctx context.Context, tx *bench.TxX, aid, tid, bid, delta int, hid int64) error {
	updateAccount, _ := w.sql.Query("workload_tx_tpcb", "update_account")
	getBalance, _ := w.sql.Query("workload_tx_tpcb", "get_balance")
	updateTeller, _ := w.sql.Query("workload_tx_tpcb", "update_teller")
	updateBranch, _ := w.sql.Query("workload_tx_tpcb", "update_branch")
	insertHistory, _ := w.sql.Query("workload_tx_tpcb", "insert_history")

	if err := tx.Exec(ctx, updateAccount, map[string]any{"aid": aid, "delta": delta}); err != nil {
		return err
	}

	abalance, err := tx.QueryValue(ctx, getBalance, map[string]any{"aid": aid})
	if err != nil {
		return err
	}

	if abalance == nil {
		return fmt.Errorf("%w: %d", errAccountNotFound, aid)
	}

	if err := tx.Exec(ctx, updateTeller, map[string]any{"tid": tid, "delta": delta}); err != nil {
		return err
	}

	if err := tx.Exec(ctx, updateBranch, map[string]any{"bid": bid, "delta": delta}); err != nil {
		return err
	}

	return tx.Exec(ctx, insertHistory, map[string]any{"hid": hid, "tid": tid, "bid": bid, "aid": aid, "delta": delta})
}

// iterateProcs executes the stored procedure tpcb_transaction as a single
// server-side round-trip per iteration (workload_procs section). No client-side
// BeginTx wraps the call: the procedure body runs the whole 5-statement
// transaction on the server (postgres' implicit statement transaction; mysql's
// procedure manages its own START TRANSACTION ... COMMIT). By design the procs
// variant therefore emits no client-side commit/rollback metric — the commit
// happens inside the procedure — so its metric shape differs from tpcb/tx.
func (w *workload) iterateProcs(ctx context.Context, b *bench.Bench, aid, tid, bid, delta int, hid int64) error {
	return b.Step("workload", func() error {
		return bench.Retry0(ctx, w.retryPolicy, func() error {
			return b.Exec(ctx, w.procQuery, map[string]any{
				"p_aid": aid, "p_tid": tid, "p_bid": bid, "p_delta": delta, "p_hid": hid,
			})
		})
	})
}

func (*workload) Teardown(ctx context.Context, b *bench.Bench) error {
	b.StepEnd("workload")

	return nil
}

// --- config helpers ---

func branches(s int64) int64 { return s }
func tellers(s int64) int64  { return 10 * s }
func accounts(s int64) int64 { return 100_000 * s }

func resolveIsolation(dt bench.DriverTypeName, override bench.TxIsolationName) bench.TxIsolationName {
	if override != "" {
		return override
	}

	switch dt {
	case bench.DriverPicodata:
		return bench.IsoNone
	case bench.DriverYDB:
		return bench.IsoSerializable
	default:
		return bench.IsoReadCommitted
	}
}

// procsSupported reports whether the procs variant can run on dt: postgres and
// mysql carry the create_procedures + workload_procs sections, and noop is
// allowed for offline overhead runs. Everything else (picodata, ydb, csv, ...)
// is rejected at setup with errProcsDriverUnsupported.
func procsSupported(dt bench.DriverTypeName) bool {
	switch dt {
	case bench.DriverPostgres, bench.DriverMySQL, bench.DriverNoop:
		return true
	default:
		return false
	}
}

func sqlFile(dt bench.DriverTypeName, override string) string {
	if override != "" {
		return override
	}

	switch dt {
	case bench.DriverMySQL:
		return "mysql.sql"
	case bench.DriverPicodata:
		return "pico.sql"
	case bench.DriverYDB:
		return "ydb.sql"
	default:
		return "pg.sql"
	}
}

func mustLoadSQL(dt bench.DriverTypeName, override string) *bench.SQL {
	s, err := bench.LoadSQL(preset, sqlFile(dt, override))
	if err != nil {
		panic(err)
	}

	return s
}

func (w *workload) resolveSQLQueries() error {
	if err := validateSQL(w.sql); err != nil {
		return err
	}

	if w.variant != "procs" {
		return nil
	}

	proc, ok := w.sql.Query("workload_procs", "tpcb_transaction")
	if !ok {
		return fmt.Errorf("%w workload_procs/tpcb_transaction", errProcsMissingQuery)
	}

	w.procQuery = proc

	return nil
}

// validateSQL asserts the schema sections and named transaction queries the
// workload needs are present before measured execution, so a custom SQL file
// that omits TPC-B statements fails with a named missing query/section instead
// of degrading into successful noop iterations.
func validateSQL(sql *bench.SQL) error {
	for _, name := range requiredSetupSections {
		if len(sql.Section(name)) == 0 {
			return fmt.Errorf("%w %q", errMissingSection, name)
		}
	}

	for _, q := range requiredTxQueries {
		body, ok := sql.Query(q.section, q.query)
		if !ok {
			return fmt.Errorf("%w %s/%s", errMissingQuery, q.section, q.query)
		}

		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("%w %s/%s", errEmptyQuery, q.section, q.query)
		}
	}

	return nil
}

// --- per-VU tx-time generators ---

type vuState struct {
	aid, tid, bid, delta *rand.Rand
	hid                  atomic.Int64
}

func (w *workload) vuState(vuid uint64) *vuState {
	if v, ok := w.vuStates.Load(vuid); ok {
		vs, _ := v.(*vuState) //nolint:errcheck // vuStates only stores *vuState values

		return vs
	}

	vs := &vuState{
		aid:   rand.New(rand.NewPCG(seedOf("aid", vuid), seedOf("aid", vuid))),     //nolint:gosec // G404: benchmark RNG
		tid:   rand.New(rand.NewPCG(seedOf("tid", vuid), seedOf("tid", vuid))),     //nolint:gosec // G404: benchmark RNG
		bid:   rand.New(rand.NewPCG(seedOf("bid", vuid), seedOf("bid", vuid))),     //nolint:gosec // G404: benchmark RNG
		delta: rand.New(rand.NewPCG(seedOf("delta", vuid), seedOf("delta", vuid))), //nolint:gosec // G404: benchmark RNG
	}
	vs.hid.Store(int64(vuid) * 1_000_000_000) //nolint:gosec // G115: value bounded by scale factor, no overflow path
	actual, _ := w.vuStates.LoadOrStore(vuid, vs)
	stored, _ := actual.(*vuState) //nolint:errcheck // vuStates only stores *vuState values

	return stored
}

func (v *vuState) nextHid() int64 { return v.hid.Add(1) }

// txParams draws one iteration's TPC-B transaction parameters. Shared by the
// tx and procs variants so identical VUs draw identical sequences.
func (v *vuState) txParams(scale int64) (aid, tid, bid, delta int, hid int64) {
	return v.aid.IntN(int(accounts(scale))) + 1,
		v.tid.IntN(int(tellers(scale))) + 1,
		v.bid.IntN(int(branches(scale))) + 1,
		v.delta.IntN(10001) - 5000,
		v.nextHid()
}

// seedOf mirrors tpcb_common.seedOf: a per-VU, per-slot offset so concurrent VUs
// draw independent sequences.
func seedOf(slot string, vuid uint64) uint64 {
	var h uint32
	for _, c := range slot {
		h = h*131 + uint32(c) //nolint:gosec // G115: value bounded by scale factor, no overflow path
	}

	return (vuid * 0x9e3779b9) ^ uint64(h)
}

func (w *workload) retryCounter(b *bench.Bench) *bench.Metric {
	w.retryMetricOnce.Do(func() { w.retryMetric = b.Counter("tpcb_retry_attempts") })

	return w.retryMetric
}

// --- typed insert requests (plain Go row formulas) ---

// branchesRequest builds the typed insert request for pgbench_branches.
// bid is the 1-based row counter, bbalance is 0, filler is a fixed-width
// [A-Za-z] string. Preserves the legacy table name, method (NATIVE), and
// seedBranches derivation.
func branchesRequest(scale int64, workers int) *driver.InsertRequest {
	root := gen.New(seedBranches)

	return &driver.InsertRequest{
		Table: "pgbench_branches", Method: driver.InsertNative, Workers: workers,
		Source: branchesSource(root, branches(scale)),
	}
}

func tellersRequest(scale int64, workers int) *driver.InsertRequest {
	root := gen.New(seedTellers)

	return &driver.InsertRequest{
		Table: "pgbench_tellers", Method: driver.InsertNative, Workers: workers,
		Source: tellersSource(root, tellers(scale)),
	}
}

func accountsRequest(scale int64, workers int) *driver.InsertRequest {
	root := gen.New(seedAccounts)

	return &driver.InsertRequest{
		Table: "pgbench_accounts", Method: driver.InsertNative, Workers: workers,
		Source: accountsSource(root, accounts(scale)),
	}
}

// branchesSource returns the indexed source for pgbench_branches. Each
// row's bid is its 1-based entity index; the filler column is the only
// random field, so it owns its own gen.Field under the versioned domain.
//
//nolint:dupl // each table's load formula is kept explicit for readability
func branchesSource(root gen.Root, totalRows int64) *gen.IndexedSource {
	fillerField := root.Domain("tpcb/branches@1").Field("filler")

	b := gen.NewSchemaBuilder()
	bidCol := b.Int64("bid")
	bbalanceCol := b.Int64("bbalance")
	fillerCol := b.Bytes("filler", branchesFiller)
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(bidCol, int64(entity)+1) //nolint:gosec // G115: bounded by totalRows
		r.SetInt64(bbalanceCol, 0)

		dst, err := r.Bytes(fillerCol, branchesFiller)
		if err != nil {
			return err
		}

		draw := fillerField.At(entity)
		gen.Alpha.Fill(&draw, dst)

		return nil
	}

	return gen.NewIndexedSource(schema, root, "tpcb/branches@1", totalRows, 64, fn)
}

// tellersSource returns the indexed source for pgbench_tellers. bid fans
// out as floor(entity / tellersPerBranch) + 1, matching the legacy DIV
// expression; tid is the 1-based entity index, tbalance is 0.
//
//nolint:dupl // each table's load formula is kept explicit for readability
func tellersSource(root gen.Root, totalRows int64) *gen.IndexedSource {
	fillerField := root.Domain("tpcb/tellers@1").Field("filler")

	b := gen.NewSchemaBuilder()
	tidCol := b.Int64("tid")
	bidCol := b.Int64("bid")
	tbalanceCol := b.Int64("tbalance")
	fillerCol := b.Bytes("filler", tellersFiller)
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(tidCol, int64(entity)+1)                          //nolint:gosec // G115: bounded by totalRows
		r.SetInt64(bidCol, int64(entity/uint64(tellersPerBranch))+1) //nolint:gosec // G115: bounded
		r.SetInt64(tbalanceCol, 0)

		dst, err := r.Bytes(fillerCol, tellersFiller)
		if err != nil {
			return err
		}

		draw := fillerField.At(entity)
		gen.Alpha.Fill(&draw, dst)

		return nil
	}

	return gen.NewIndexedSource(schema, root, "tpcb/tellers@1", totalRows, 64, fn)
}

// accountsSource returns the indexed source for pgbench_accounts. bid fans
// out as floor(entity / accountsPerBranch) + 1; aid is the 1-based entity
// index, abalance is 0.
//
//nolint:dupl // each table's load formula is kept explicit for readability
func accountsSource(root gen.Root, totalRows int64) *gen.IndexedSource {
	fillerField := root.Domain("tpcb/accounts@1").Field("filler")

	b := gen.NewSchemaBuilder()
	aidCol := b.Int64("aid")
	bidCol := b.Int64("bid")
	abalanceCol := b.Int64("abalance")
	fillerCol := b.Bytes("filler", accountFiller)
	schema := b.Build()

	fn := func(r gen.Row, entity uint64) error {
		r.SetInt64(aidCol, int64(entity)+1)                           //nolint:gosec // G115: bounded by totalRows
		r.SetInt64(bidCol, int64(entity/uint64(accountsPerBranch))+1) //nolint:gosec // G115: bounded
		r.SetInt64(abalanceCol, 0)

		dst, err := r.Bytes(fillerCol, accountFiller)
		if err != nil {
			return err
		}

		draw := fillerField.At(entity)
		gen.Alpha.Fill(&draw, dst)

		return nil
	}

	return gen.NewIndexedSource(schema, root, "tpcb/accounts@1", totalRows, 64, fn)
}
