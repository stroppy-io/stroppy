//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
)

// TestTpcbSmokeIntegration is the Stage E end-to-end smoke: it proves the
// datagen framework can seed TPC-B's three dimension tables (branches,
// tellers, accounts) from Go struct-literal InsertSpecs, and that the
// resulting data supports TPC-B balance-update transactions with the
// sum-of-balances invariant holding.
//
// Scale: SF=0.01 → 1 branch, 1 teller, 1000 accounts. Small enough to keep
// the test fast while preserving every structural property of the spec.
func TestTpcbSmokeIntegration(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	tpcbCreateTables(t, pool)

	branchesSpec := tpcbBranchesSpec()
	tellersSpec := tpcbTellersSpec()
	accountsSpec := tpcbAccountsSpec()

	tpcbRunSpec(t, pool, branchesSpec, "branches", tpcbBranchesColumns)
	tpcbRunSpec(t, pool, tellersSpec, "tellers", tpcbTellersColumns)
	tpcbRunSpec(t, pool, accountsSpec, "accounts", tpcbAccountsColumns)

	if got := CountRows(t, pool, "branches"); got != tpcbBranches {
		t.Fatalf("branches: row count = %d, want %d", got, tpcbBranches)
	}
	if got := CountRows(t, pool, "tellers"); got != tpcbTellers {
		t.Fatalf("tellers: row count = %d, want %d", got, tpcbTellers)
	}
	if got := CountRows(t, pool, "accounts"); got != tpcbAccounts {
		t.Fatalf("accounts: row count = %d, want %d", got, tpcbAccounts)
	}

	// Fixed seed: transactions are reproducible but not load-bearing; the
	// invariant is what we assert.
	rng := rand.New(rand.NewPCG(0xAB1BA5, 0xC0FFEE)) //nolint:gosec // deterministic test
	tpcbRunTransactions(t, pool, rng, tpcbTxCount)

	tpcbAssertInvariants(t, pool)

	t.Run("Determinism", func(t *testing.T) {
		// Running the seed step twice with a fresh schema between must
		// produce byte-identical rows when selected in PK order. This
		// verifies the seekable-by-construction guarantee for the TPC-B
		// seed specs.
		first := tpcbSeedAndSnapshot(t, pool)
		second := tpcbSeedAndSnapshot(t, pool)

		if !reflect.DeepEqual(first, second) {
			t.Fatalf("seed determinism: snapshots differ\n first=%v\n second=%v",
				first, second)
		}
	})
}

// ---------- DDL ----------

func tpcbCreateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ddls := []string{
		`CREATE TABLE branches (
			bid       int PRIMARY KEY,
			bbalance  numeric,
			filler    char(88)
		)`,
		`CREATE TABLE tellers (
			tid       int PRIMARY KEY,
			bid       int,
			tbalance  numeric,
			filler    char(84)
		)`,
		`CREATE TABLE accounts (
			aid       int PRIMARY KEY,
			bid       int,
			abalance  numeric,
			filler    char(84)
		)`,
		`CREATE TABLE history (
			tid     int,
			bid     int,
			aid     int,
			delta   numeric,
			mtime   timestamp,
			filler  char(22)
		)`,
	}
	for _, ddl := range ddls {
		if _, err := pool.Exec(context.Background(), ddl); err != nil {
			t.Fatalf("create table: %v (ddl=%q)", err, ddl)
		}
	}
}

// ---------- Scale + shape ----------

const (
	// SF=0.01: 1 branch × 1 teller × 1000 accounts. TPC-B's spec ratio is
	// 1:10:100_000 per unit, but structural properties hold at any scale;
	// the small scale keeps the test under the per-PR budget.
	tpcbBranches = int64(1)
	tpcbTellers  = int64(1)
	tpcbAccounts = int64(1000)

	tpcbTxCount = 10

	// Balance swing bounded so the invariant equals exactly 10 delta sums.
	tpcbDeltaMin = int64(-100)
	tpcbDeltaMax = int64(100)

	tpcbBranchesFiller = "BRANCH-FILLER-" // padded to 88 in the spec
	tpcbTellersFiller  = "TELLER-FILLER-" // padded to 84
	tpcbAccountsFiller = "ACCOUNT-FILL-"  // padded to 84
)

var (
	tpcbBranchesColumns = []string{"bid", "bbalance", "filler"}
	tpcbTellersColumns  = []string{"tid", "bid", "tbalance", "filler"}
	tpcbAccountsColumns = []string{"aid", "bid", "abalance", "filler"}
)

// ---------- Spec builders ----------

// tpcbBranchesSpec yields 1 row: bid=1, bbalance=0, filler (padded).
func tpcbBranchesSpec() *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("bid", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		attrOf("bbalance", litOf(int64(0))),
		attrOf("filler", litOf(padAscii(tpcbBranchesFiller, 88))),
	}
	return &dgproto.InsertSpec{
		Table: "branches",
		Seed:  0x7B01B,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "branches", Size: tpcbBranches},
			Attrs:       attrs,
			ColumnOrder: tpcbBranchesColumns,
		},
	}
}

// tpcbTellersSpec yields 1 row per branch (scale-invariant: 10 tellers
// per branch at full SF, reduced to 1 at SF=0.01): tid=1, bid=1,
// tbalance=0, filler.
func tpcbTellersSpec() *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("tid", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		attrOf("bid", litOf(int64(1))),
		attrOf("tbalance", litOf(int64(0))),
		attrOf("filler", litOf(padAscii(tpcbTellersFiller, 84))),
	}
	return &dgproto.InsertSpec{
		Table: "tellers",
		Seed:  0x7E11E,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "tellers", Size: tpcbTellers},
			Attrs:       attrs,
			ColumnOrder: tpcbTellersColumns,
		},
	}
}

// tpcbAccountsSpec yields 1000 rows all attached to branch 1.
func tpcbAccountsSpec() *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("aid", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		attrOf("bid", litOf(int64(1))),
		attrOf("abalance", litOf(int64(0))),
		attrOf("filler", litOf(padAscii(tpcbAccountsFiller, 84))),
	}
	return &dgproto.InsertSpec{
		Table: "accounts",
		Seed:  0xACC07,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "accounts", Size: tpcbAccounts},
			Attrs:       attrs,
			ColumnOrder: tpcbAccountsColumns,
		},
	}
}

// padAscii right-pads s with spaces to exactly width bytes (or truncates).
// TPC-B's filler columns are fixed-width CHAR, and Postgres stores CHAR(n)
// with trailing spaces anyway, but we emit the explicit padded string to
// keep round-trips byte-stable.
func padAscii(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	buf := make([]byte, width)
	copy(buf, s)
	for i := len(s); i < width; i++ {
		buf[i] = ' '
	}
	return string(buf)
}

// ---------- Runtime drive + COPY ----------

// tpcbDrain materializes a spec to a [][]any via runtime.NewRuntime.
func tpcbDrain(t *testing.T, spec *dgproto.InsertSpec) [][]any {
	t.Helper()

	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		t.Fatalf("NewRuntime(%s): %v", spec.GetTable(), err)
	}

	var rows [][]any
	for {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			return rows
		}
		if err != nil {
			t.Fatalf("Next(%s): %v", spec.GetTable(), err)
		}
		out := make([]any, len(row))
		copy(out, row)
		rows = append(rows, out)
	}
}

// tpcbRunSpec drains the spec and bulk-loads via pgx.CopyFrom.
func tpcbRunSpec(
	t *testing.T,
	pool *pgxpool.Pool,
	spec *dgproto.InsertSpec,
	table string,
	columns []string,
) {
	t.Helper()

	rows := tpcbDrain(t, spec)
	if _, err := pool.CopyFrom(
		context.Background(),
		pgx.Identifier{table},
		columns,
		pgx.CopyFromRows(rows),
	); err != nil {
		t.Fatalf("CopyFrom(%s): %v", table, err)
	}
}

// ---------- TPC-B transactions ----------

// tpcbRunTransactions drives `count` balance-update transactions. Each
// transaction mirrors the TPC-B spec: update one account, one teller, one
// branch, then log in history. Runs under a single explicit tx so that
// aborting halfway leaves no torn state.
func tpcbRunTransactions(t *testing.T, pool *pgxpool.Pool, rng *rand.Rand, count int) {
	t.Helper()

	ctx := context.Background()
	for i := range count {
		aid := rng.Int64N(tpcbAccounts) + 1
		delta := rng.Int64N(tpcbDeltaMax-tpcbDeltaMin+1) + tpcbDeltaMin

		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("tx %d: begin: %v", i, err)
		}

		if _, err := tx.Exec(ctx,
			`UPDATE accounts SET abalance = abalance + $1 WHERE aid = $2`,
			delta, aid,
		); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("tx %d: update accounts: %v", i, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE tellers SET tbalance = tbalance + $1 WHERE tid = 1`,
			delta,
		); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("tx %d: update tellers: %v", i, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE branches SET bbalance = bbalance + $1 WHERE bid = 1`,
			delta,
		); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("tx %d: update branches: %v", i, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO history (tid, bid, aid, delta, mtime, filler)
			 VALUES (1, 1, $1, $2, now(), 'X')`,
			aid, delta,
		); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("tx %d: insert history: %v", i, err)
		}

		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("tx %d: commit: %v", i, err)
		}
	}
}

// ---------- Invariants ----------

func tpcbAssertInvariants(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	// history row count equals tx count.
	if got := CountRows(t, pool, "history"); got != int64(tpcbTxCount) {
		t.Fatalf("history: row count = %d, want %d", got, tpcbTxCount)
	}

	// Read all four sums in one round trip.
	var branchSum, tellerSum, accountSum, historySum int64
	err := pool.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT SUM(bbalance) FROM branches), 0)::int8,
			COALESCE((SELECT SUM(tbalance) FROM tellers), 0)::int8,
			COALESCE((SELECT SUM(abalance) FROM accounts), 0)::int8,
			COALESCE((SELECT SUM(delta)    FROM history), 0)::int8
	`).Scan(&branchSum, &tellerSum, &accountSum, &historySum)
	if err != nil {
		t.Fatalf("invariant sums query: %v", err)
	}

	if branchSum != historySum {
		t.Fatalf("invariant: SUM(branches.bbalance)=%d != SUM(history.delta)=%d",
			branchSum, historySum)
	}
	if tellerSum != historySum {
		t.Fatalf("invariant: SUM(tellers.tbalance)=%d != SUM(history.delta)=%d",
			tellerSum, historySum)
	}
	if accountSum != historySum {
		t.Fatalf("invariant: SUM(accounts.abalance)=%d != SUM(history.delta)=%d",
			accountSum, historySum)
	}
}

// ---------- Determinism snapshot ----------

// tpcbSnapshot holds a deterministic read of every seeded row, selected
// in PK order so the slices compare exactly across runs.
type tpcbSnapshot struct {
	Branches [][]any
	Tellers  [][]any
	Accounts [][]any
}

// tpcbSeedAndSnapshot resets the schema, recreates tables, runs the seed
// once more, and reads every row back in PK order.
func tpcbSeedAndSnapshot(t *testing.T, pool *pgxpool.Pool) tpcbSnapshot {
	t.Helper()

	ResetSchema(t, pool)
	tpcbCreateTables(t, pool)

	tpcbRunSpec(t, pool, tpcbBranchesSpec(), "branches", tpcbBranchesColumns)
	tpcbRunSpec(t, pool, tpcbTellersSpec(), "tellers", tpcbTellersColumns)
	tpcbRunSpec(t, pool, tpcbAccountsSpec(), "accounts", tpcbAccountsColumns)

	return tpcbSnapshot{
		Branches: tpcbFetch(t, pool, "SELECT bid, bbalance::text, filler FROM branches ORDER BY bid"),
		Tellers:  tpcbFetch(t, pool, "SELECT tid, bid, tbalance::text, filler FROM tellers ORDER BY tid"),
		Accounts: tpcbFetch(t, pool, "SELECT aid, bid, abalance::text, filler FROM accounts ORDER BY aid"),
	}
}

// tpcbFetch reads all rows from query into [][]any. Numerics are cast to
// text on the SQL side to sidestep pgx.Numeric's opaque internal
// representation; equality is then a plain string compare.
func tpcbFetch(t *testing.T, pool *pgxpool.Pool, query string) [][]any {
	t.Helper()

	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("fetch %q: %v", query, err)
	}
	defer rows.Close()

	var out [][]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatalf("fetch %q: values: %v", query, err)
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("fetch %q: rows.Err: %v", query, err)
	}
	return out
}
