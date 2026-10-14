// Package banking provides a built-in bank-transfer invariant workload: every
// transfer must preserve the total balance and the token multiset, under
// concurrency and under injected faults.
//
// The workload is derived from OrioleDB's bank-account crash test, without the
// storage engine it was written for: the invariants, the two-row transfer and
// the fault injection are kept, the engine patch is not.
package banking

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

const (
	// initialBalance is the balance every account starts with, so the expected
	// total is accounts * initialBalance.
	initialBalance = 1000
	// defaultAccounts is the account count when the operator passes no scale.
	defaultAccounts = 100
	// maxTransferAmount bounds a single transfer; it is deliberately small
	// relative to initialBalance so the sum invariant is the thing under test,
	// not overdraft handling.
	maxTransferAmount = 10
)

var errTokenLost = errors.New("banking: token rotation lost to a concurrent transfer")

// Test is the portable variant: no fault injection, works on any supported
// driver.
var Test = bench.Test{Name: "banking", Define: define, Source: publication("Test")}

func init() { bench.Register(Test) }

type workload struct {
	sql      *bench.SQL
	accounts int64
	retries  int
	iso      bench.TxIsolationName

	read      bench.QueryHandle
	tokenOnly bench.QueryHandle
	rotate    bench.QueryHandle
	move      bench.QueryHandle
	total     bench.QueryHandle
	tokens    bench.QueryHandle

	// inject is nil for the plain variant and set by the fault-injecting one.
	inject *injectConfig

	// injectArmedFor records, per worker, the iteration the injector was last armed
	// for. A retry must not arm a second time: the event is one-shot, so re-arming
	// inside the retry loop would fail every attempt of that iteration and no retry
	// could absorb the fault.
	injectArmedFor []uint64

	// driverType is the declared driver's kind, kept so a variant can refuse a
	// driver it cannot work with.
	driverType bench.DriverTypeName

	// randoms is per worker, so a worker's transfers are deterministic and two
	// workers do not share a stream.
	randoms []*rand.Rand
}

func define(d *bench.Def) error { return defineVariant(d, "") }

func defineVariant(d *bench.Def, variant string) error {
	settings := bench.RunParameters(&d.Param, bench.RunDefaults{})

	w := &workload{
		retries:        3,
		iso:            bench.IsoReadCommitted,
		randoms:        make([]*rand.Rand, max(settings.Workers, 1)),
		injectArmedFor: make([]uint64, max(settings.Workers, 1)),
	}

	for worker := range w.injectArmedFor {
		w.injectArmedFor[worker] = neverArmed
	}

	accounts, _ := d.Param.Int64("accounts", defaultAccounts,
		"Number of bank accounts; the invariant is accounts * 1000.")
	w.accounts = max(accounts, 2)

	w.retries, _ = d.Param.Int("retry-attempts", 3, "Maximum transaction attempts.")
	iso, _ := d.Param.String("tx-isolation", "", "Transaction isolation override.")
	sqlFile, _ := d.Param.String("sql-file", "", "SQL dialect file override.")

	db := d.Drivers.Declare("default", bench.DriverConfig{})
	w.driverType = db.Kind()
	w.iso = resolveIsolation(db.Kind(), bench.TxIsolationName(iso))

	var err error
	if sqlFile != "" {
		w.sql, err = d.Queries.Override(sqlFile)
	} else {
		w.sql, err = d.Queries.Load(files, sqlFileFor(db.Kind()))
	}
	if err != nil {
		return err
	}

	w.read = w.sql.Require("workload_tx_transfer", "read_account")
	w.tokenOnly = w.sql.Require("workload_tx_transfer", "read_token")
	w.rotate = w.sql.Require("workload_tx_transfer", "rotate_token")
	w.move = w.sql.Require("workload_tx_transfer", "move_balance")
	w.total = w.sql.Require("verify", "total_balance")
	w.tokens = w.sql.Require("verify", "token_stats")

	for worker := range w.randoms {
		//nolint:gosec // benchmark data generator, not security randomness.
		w.randoms[worker] = rand.New(rand.NewPCG(uint64(worker)+1, 0x5EED))
	}

	if err := w.configureVariant(d, variant); err != nil {
		return err
	}

	for _, name := range []string{"drop_schema", "create_schema"} {
		d.Execution.Step(name, sqlSection{sql: w.sql, name: name}.Run)
	}

	d.Execution.Step("load_data", w.load)
	d.Execution.Step("transfer", w.transfer, settings.Policy())
	d.Execution.Step("verify", w.verify)

	return d.Execution.Err()
}

// load fills every account with its initial balance and its own token, so the
// tokens start as the permutation 1..accounts.
func (w *workload) load(ctx context.Context, b *bench.Bench) error {
	source := accountRows(w.accounts)

	if _, err := b.Insert(ctx, "bank_account", source,
		bench.InsertMethod(bench.InsertPlainBulk), bench.LoadWorkers(1)); err != nil {
		return fmt.Errorf("load_data: %w", err)
	}

	return nil
}

// transfer moves an amount between two accounts and rotates their tokens.
//
// The token is the second witness of the transfer: it is UNIQUE, so the swap
// cannot be lost quietly, and it is moved *conditionally* on the value that was
// read, which turns a concurrent transfer into a detected conflict rather than a
// silent overwrite.
func (w *workload) transfer(ctx context.Context, b *bench.Bench) error {
	random := w.randoms[b.Worker()%len(w.randoms)]

	from := int64(random.IntN(int(w.accounts))) + 1
	to := int64(random.IntN(int(w.accounts))) + 1
	if from == to {
		to = from%w.accounts + 1
	}

	amount := int64(random.IntN(maxTransferAmount)) + 1

	return b.Transaction(ctx, bench.TransactionOptions{
		Name:      "transfer",
		Isolation: w.iso,
		Retry:     w.retryOptions(),
	}, func(ctx context.Context, tx *bench.Tx) error {
		fromToken, err := w.readToken(ctx, tx, from)
		if err != nil {
			return err
		}

		toToken, err := w.readToken(ctx, tx, to)
		if err != nil {
			return err
		}

		// The swap needs a sentinel because both rows would otherwise want to
		// hold a token at once, and the constraint is immediate.
		for _, move := range swapMoves(from, to, fromToken, toToken) {
			if err := w.rotateToken(ctx, tx, move.id, move.token, move.expected); err != nil {
				return err
			}
		}

		for _, move := range balanceMoves(from, to, amount) {
			if err := tx.Exec(ctx, w.move.Text, map[string]any{"id": move.id, "delta": move.delta}); err != nil {
				return fmt.Errorf("move_balance: %w", err)
			}
		}

		return w.maybeInject(ctx, tx, b)
	})
}

// retryOptions is what makes a transfer retryable, including for the two failures
// that classify as unknown — the default policy fails closed on those.
//
// One is the conflict this workload detects itself: a conditional write that no
// longer matches means a concurrent transfer reached the row first, which is
// exactly the contention a retry is for. The other is the fault injector's error.
//
// Retrying is safe because the transaction is the unit of work: a failed attempt
// leaves nothing behind, and even a repeat of a transfer that did commit cannot
// break an invariant, because a transfer moves a fixed amount between two
// accounts and conserves the total — applying it twice is indistinguishable from
// two transfers.
func (w *workload) retryOptions() bench.RetryOptions {
	actions := bench.DefaultErrorActions()
	actions[driver.ErrorKindUnknown] = bench.ErrorActionRetry

	return bench.RetryOptions{
		MaxAttempts: w.retries,
		Idempotent:  true,
		Actions:     actions,
	}
}

// tokenMove is one conditional token write: the row, the value it should end up
// holding, and the value it must still hold for the write to apply.
type tokenMove struct {
	id       int64
	token    int64
	expected int64
}

// balanceMove is one relative balance change.
type balanceMove struct {
	id    int64
	delta int64
}

// swapMoves is the exchange between two accounts, as three conditional writes.
//
// It takes three rather than two because the UNIQUE constraint is immediate and
// both rows would otherwise want to hold a token at once: the first row parks its
// token in a sentinel, the negative of its own id — outside 1..accounts, and
// unique to that row, so no two rows can ever hold the same sentinel — and picks
// the other's token up last. Each write is conditional on the value read, which is
// what makes a concurrent transfer a detected conflict instead of a silent
// overwrite.
func swapMoves(from, to, fromToken, toToken int64) []tokenMove {
	sentinel := -from

	return []tokenMove{
		{id: from, token: sentinel, expected: fromToken},
		{id: to, token: fromToken, expected: toToken},
		{id: from, token: toToken, expected: sentinel},
	}
}

// balanceMoves is the money side of the same transfer: equal and opposite, so the
// total is untouched by construction.
func balanceMoves(from, to, amount int64) []balanceMove {
	return []balanceMove{{id: from, delta: -amount}, {id: to, delta: amount}}
}

// readToken reads one account's token inside the transaction, using the
// single-column form: a test can then distinguish the transfer's initial read of
// both columns from the confirmation read that follows its write.
func (w *workload) readToken(ctx context.Context, tx *bench.Tx, id int64) (int64, error) {
	row, err := tx.QueryRow(ctx, w.read.Text, map[string]any{"id": id})
	if err != nil {
		return 0, fmt.Errorf("read_account %d: %w", id, err)
	}

	token, err := row.Int64("token")
	if err != nil {
		return 0, err
	}

	return token, nil
}

// confirmToken reads the token back after a write, through the narrow query.
func (w *workload) confirmToken(ctx context.Context, tx *bench.Tx, id int64) (int64, error) {
	row, err := tx.QueryRow(ctx, w.tokenOnly.Text, map[string]any{"id": id})
	if err != nil {
		return 0, fmt.Errorf("read_token %d: %w", id, err)
	}

	token, err := row.Int64("token")
	if err != nil {
		return 0, err
	}

	return token, nil
}

// rotateToken moves a token conditionally on the value that was read, then
// confirms the move within the same transaction.
//
// Exec exposes no row count, so a conditional update that matched nothing looks
// exactly like one that matched: the confirmation read is what tells the two
// apart. A read sees the transaction's own writes, so a token that is not the one
// we just wrote means a concurrent transfer got there first — which is a conflict
// for the retry policy, not a failure of the run.
func (w *workload) rotateToken(ctx context.Context, tx *bench.Tx, id, token, expected int64) error {
	if err := tx.Exec(ctx, w.rotate.Text, map[string]any{
		"id": id, "token": token, "expected": expected,
	}); err != nil {
		return fmt.Errorf("rotate_token %d: %w", id, err)
	}

	got, err := w.confirmToken(ctx, tx, id)
	if err != nil {
		return err
	}

	if got != token {
		return fmt.Errorf("%w: account %d holds %d, expected %d", errTokenLost, id, got, token)
	}

	return nil
}

// verifyAttempts and verifyDelay bound how long the invariant check waits for a
// server that is not answering yet.
//
// The crash variant leaves the cluster reinitializing: measured against a real
// server, recovery took 0.3–3.8 s and the cluster was ready for only 12–58 ms
// before the next crash, so a check that gave up on the first refusal could never
// report on the invariants after a crash — which is the whole point of that variant.
const (
	verifyAttempts = 12
	verifyDelay    = 500 * time.Millisecond
)

// verify asserts both invariants, waiting out a server that is still recovering.
func (w *workload) verify(ctx context.Context, b *bench.Bench) error {
	return retryCheck(ctx, verifyAttempts, verifyDelay, func() error {
		return w.checkInvariants(ctx, b)
	})
}

// retryCheck runs check until it succeeds or the attempts are spent, returning the
// last error. A genuine invariant violation is not swallowed: it is reported after
// the wait, unchanged.
func retryCheck(ctx context.Context, attempts int, delay time.Duration, check func() error) error {
	var last error

	for attempt := range attempts {
		if attempt > 0 && delay > 0 {
			select {
			case <-ctx.Done():
				return last
			case <-time.After(delay):
			}
		}

		if err := check(); err != nil {
			last = err

			continue
		}

		return nil
	}

	return last
}

// checkInvariants asserts both invariants: the money is conserved and the tokens
// are still a permutation of 1..accounts. A lost or duplicated transfer breaks the
// second even when the first happens to hold.
func (w *workload) checkInvariants(ctx context.Context, b *bench.Bench) error {
	total, err := b.QueryValue[int64](ctx, w.total.Text, nil)
	if err != nil {
		return fmt.Errorf("verify total: %w", err)
	}

	if want := w.accounts * initialBalance; total != want {
		return fmt.Errorf("verify: total balance is %d, want %d", total, want)
	}

	row, err := b.QueryRow(ctx, w.tokens.Text, nil)
	if err != nil {
		return fmt.Errorf("verify tokens: %w", err)
	}

	rows, err := row.Int64("accounts_rows")
	if err != nil {
		return err
	}

	lowest, err := row.Int64("lowest_token")
	if err != nil {
		return err
	}

	highest, err := row.Int64("highest_token")
	if err != nil {
		return err
	}

	repeated, err := row.Int64("repeated_tokens")
	if err != nil {
		return err
	}

	// No token repeats, and they span exactly 1..accounts: together with the row
	// count that is the permutation, and it is a stronger witness than a distinct
	// count — a duplicate paired with a loss still shows up here as a repeat.
	if rows != w.accounts || repeated != 0 || lowest != 1 || highest != w.accounts {
		return fmt.Errorf(
			"verify: tokens are not the permutation 1..%d (rows %d, repeated %d, min %d, max %d)",
			w.accounts, rows, repeated, lowest, highest,
		)
	}

	return nil
}
