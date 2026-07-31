// Package tpcb is the Go-native port of workloads/tpcb/tx.ts: pgbench's canonical
// 5-statement TPC-B transaction under one explicit transaction per iteration.
// Shares load/config with the procs variant (not ported here). Supports pg/mysql/
// picodata/ydb; isolation + SQL file are selected by driver type.
package tpcb

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
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
	sql        *bench.SQL
	driverType bench.DriverTypeName
	iso        bench.TxIsolationName
	scale      int64

	retryMetricOnce sync.Once
	retryMetric     *bench.Metric

	vuStates sync.Map // uint64 -> *vuState
}

func init() { bench.Register(&workload{}) }

func (*workload) Name() string { return "tpcb/tx" }

func (w *workload) Setup(ctx context.Context, b *bench.Bench) error {
	w.driverType = b.DriverTypeName()

	w.scale = int64(bench.EnvInt("SCALE_FACTOR", 1))
	if w.scale < 1 {
		w.scale = 1
	}

	w.iso = resolveIsolation(w.driverType)
	w.sql = mustLoadSQL(w.driverType)

	runSection := func(name string) error {
		for _, q := range w.sql.Section(name) {
			if err := b.Exec(ctx, q, nil); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}

		return nil
	}

	steps := []struct {
		name string
		fn   func() error
	}{
		{"drop_schema", func() error { return runSection("drop_schema") }},
		{"create_schema", func() error { return runSection("create_schema") }},
		{"load_data", func() error {
			if _, err := b.InsertSpec(ctx, branchesSpec(w.scale)); err != nil {
				return err
			}

			if _, err := b.InsertSpec(ctx, tellersSpec(w.scale)); err != nil {
				return err
			}

			if _, err := b.InsertSpec(ctx, accountsSpec(w.scale)); err != nil {
				return err
			}

			return nil
		}},
		{"create_indexes", func() error { return runSection("create_indexes") }},
		{"create_foreign_keys", func() error { return runSection("create_foreign_keys") }},
		{"analyze", func() error { return runSection("analyze") }},
	}
	for _, s := range steps {
		if err := b.Step(s.name, s.fn); err != nil {
			return err
		}
	}

	b.StepBegin("workload")

	return nil
}

func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	vs := w.vuState(b.VUID())
	aid := vs.aid.IntN(int(accounts(w.scale))) + 1
	tid := vs.tid.IntN(int(tellers(w.scale))) + 1
	bid := vs.bid.IntN(int(branches(w.scale))) + 1
	delta := vs.delta.IntN(10001) - 5000
	hid := vs.nextHid()

	policy := bench.TxRetryPolicy(w.driverType, bench.TxRetryPolicyOptions{
		MaxAttempts: bench.EnvInt("RETRY_ATTEMPTS", 3),
		OnRetry:     func(int, error, bench.RetryDecision) { w.retryCounter(b).Add(1) },
	})

	updateAccount, _ := w.sql.Query("workload_tx_tpcb", "update_account")
	getBalance, _ := w.sql.Query("workload_tx_tpcb", "get_balance")
	updateTeller, _ := w.sql.Query("workload_tx_tpcb", "update_teller")
	updateBranch, _ := w.sql.Query("workload_tx_tpcb", "update_branch")
	insertHistory, _ := w.sql.Query("workload_tx_tpcb", "insert_history")

	return b.Step("workload", func() error {
		return bench.Retry0(policy, func() error {
			return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "tpcb"}, func(tx *bench.TxX) error {
				if err := tx.Exec(ctx, updateAccount, map[string]any{"aid": aid, "delta": delta}); err != nil {
					return err
				}

				abalance, err := tx.QueryValue(ctx, getBalance, map[string]any{"aid": aid})
				if err != nil {
					return err
				}

				if abalance == nil {
					return fmt.Errorf("tpc-b: account %d not found", aid)
				}

				if err := tx.Exec(ctx, updateTeller, map[string]any{"tid": tid, "delta": delta}); err != nil {
					return err
				}

				if err := tx.Exec(ctx, updateBranch, map[string]any{"bid": bid, "delta": delta}); err != nil {
					return err
				}

				return tx.Exec(ctx, insertHistory, map[string]any{"hid": hid, "tid": tid, "bid": bid, "aid": aid, "delta": delta})
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

func resolveIsolation(dt bench.DriverTypeName) bench.TxIsolationName {
	if v := bench.Env("TX_ISOLATION", ""); v != "" {
		return bench.TxIsolationName(v)
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

func sqlFile(dt bench.DriverTypeName) string {
	if v := bench.Env("SQL_FILE", ""); v != "" {
		return v
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

func mustLoadSQL(dt bench.DriverTypeName) *bench.SQL {
	s, err := bench.LoadSQL(preset, sqlFile(dt))
	if err != nil {
		panic(err)
	}

	return s
}

// --- per-VU tx-time generators ---

type vuState struct {
	aid, tid, bid, delta *rand.Rand
	hid                  atomic.Int64
}

func (w *workload) vuState(vuid uint64) *vuState {
	if v, ok := w.vuStates.Load(vuid); ok {
		return v.(*vuState)
	}

	vs := &vuState{
		aid:   rand.New(rand.NewPCG(seedOf("aid", vuid), seedOf("aid", vuid))),   //nolint:gosec // G404: benchmark RNG
		tid:   rand.New(rand.NewPCG(seedOf("tid", vuid), seedOf("tid", vuid))),   //nolint:gosec // G404: benchmark RNG
		bid:   rand.New(rand.NewPCG(seedOf("bid", vuid), seedOf("bid", vuid))),   //nolint:gosec // G404: benchmark RNG
		delta: rand.New(rand.NewPCG(seedOf("delta", vuid), seedOf("delta", vuid))), //nolint:gosec // G404: benchmark RNG
	}
	vs.hid.Store(int64(vuid) * 1_000_000_000) //nolint:gosec // G115: value bounded by scale factor, no overflow path
	actual, _ := w.vuStates.LoadOrStore(vuid, vs)

	return actual.(*vuState)
}

func (v *vuState) nextHid() int64 { return v.hid.Add(1) }

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

// --- InsertSpec builders (struct literals, what Rel/Draw compiles to) ---

func workers() *dgproto.Parallelism {
	if n := bench.EnvInt("LOAD_WORKERS", 0); n > 0 {
		return &dgproto.Parallelism{Workers: int32(n)} //nolint:gosec // G115: value bounded by scale factor, no overflow path
	}

	return nil
}

func branchesSpec(scale int64) *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Table: "pgbench_branches", Seed: seedBranches, Method: dgproto.InsertMethod_NATIVE,
		Parallelism: workers(),
		Generator: &dgproto.InsertSpec_Source{Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "branches", Size: branches(scale)},
			ColumnOrder: []string{"bid", "bbalance", "filler"},
			Attrs: []*dgproto.Attr{
				{Name: "bid", Expr: rowId()},
				{Name: "bbalance", Expr: litInt(0)},
				{Name: "filler", Expr: asciiDraw(branchesFiller)},
			},
		}},
	}
}

func tellersSpec(scale int64) *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Table: "pgbench_tellers", Seed: seedTellers, Method: dgproto.InsertMethod_NATIVE,
		Parallelism: workers(),
		Generator: &dgproto.InsertSpec_Source{Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "tellers", Size: tellers(scale)},
			ColumnOrder: []string{"tid", "bid", "tbalance", "filler"},
			Attrs: []*dgproto.Attr{
				{Name: "tid", Expr: rowId()},
				{Name: "bid", Expr: binOp(dgproto.BinOp_DIV, rowIndex(), litInt(tellersPerBranch), litInt(1))},
				{Name: "tbalance", Expr: litInt(0)},
				{Name: "filler", Expr: asciiDraw(tellersFiller)},
			},
		}},
	}
}

func accountsSpec(scale int64) *dgproto.InsertSpec {
	return &dgproto.InsertSpec{
		Table: "pgbench_accounts", Seed: seedAccounts, Method: dgproto.InsertMethod_NATIVE,
		Parallelism: workers(),
		Generator: &dgproto.InsertSpec_Source{Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "accounts", Size: accounts(scale)},
			ColumnOrder: []string{"aid", "bid", "abalance", "filler"},
			Attrs: []*dgproto.Attr{
				{Name: "aid", Expr: rowId()},
				{Name: "bid", Expr: binOp(dgproto.BinOp_DIV, rowIndex(), litInt(accountsPerBranch), litInt(1))},
				{Name: "abalance", Expr: litInt(0)},
				{Name: "filler", Expr: asciiDraw(accountFiller)},
			},
		}},
	}
}

func rowId() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{
		Op: dgproto.BinOp_ADD,
		A:  &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{}}},
		B:  litInt(1),
	}}}
}

func rowIndex() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{}}}
}

// binOp(div, a/b + c) builds (a DIV b) ADD c — the bid fan-out expressions.
func binOp(op dgproto.BinOp_Op, a, b, addC *dgproto.Expr) *dgproto.Expr {
	div := &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{Op: op, A: a, B: b}}}

	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{Op: dgproto.BinOp_ADD, A: div, B: addC}}}
}

func asciiDraw(width int) *dgproto.Expr {
	n := litInt(int64(width))

	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Ascii{Ascii: &dgproto.DrawAscii{
			MinLen: n, MaxLen: n,
			Alphabet: []*dgproto.AsciiRange{{Min: 65, Max: 90}, {Min: 97, Max: 122}},
		}},
	}}}
}

func litInt(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
}
