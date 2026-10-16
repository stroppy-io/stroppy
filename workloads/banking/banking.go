// Package banking is a bank-transfer invariant workload: every transfer must preserve
// the total balance and the token multiset, under concurrency and injected faults.
//
// Derived from OrioleDB's bank-account crash test without the storage engine it was
// written for: the invariants and the fault injection are kept, the patch is not.
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
	// initialBalance is what every account starts with, so the expected total is
	// accounts * initialBalance.
	initialBalance  = 1000
	defaultAccounts = 100
	// maxTransferAmount is kept small so the sum invariant is what is under test, not
	// overdraft handling.
	maxTransferAmount = 10
)

var errTokenLost = errors.New("banking: token rotation lost to a concurrent transfer")

const (
	defaultRetries = 3
	minAccounts    = 2
	rngSeed        = 0x5EED
)

var (
	errBalanceMismatch  = errors.New("verify: total balance")
	errTokenPermutation = errors.New("verify: tokens are not the permutation")
)

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

	inject *injectConfig

	// injectArmedFor records, per worker, the iteration the injector was last armed for.
	// A retry must not arm again: the event is one-shot, so re-arming inside the retry
	// loop would fail every attempt of that iteration and no retry could absorb it.
	injectArmedFor []uint64

	driverType bench.DriverTypeName

	// randoms is per worker, so a worker's transfers are deterministic and two workers
	// do not share a stream.
	randoms []*rand.Rand
}

func define(d *bench.Def) error { return defineVariant(d, "") }

func defineVariant(d *bench.Def, variant string) error {
	settings := bench.RunParameters(&d.Param, bench.RunDefaults{})

	w := &workload{
		retries:        defaultRetries,
		iso:            bench.IsoReadCommitted,
		randoms:        make([]*rand.Rand, max(settings.Workers, 1)),
		injectArmedFor: make([]uint64, max(settings.Workers, 1)),
	}

	for worker := range w.injectArmedFor {
		w.injectArmedFor[worker] = neverArmed
	}

	accounts, _ := d.Param.Int64("accounts", defaultAccounts,
		"Number of bank accounts; the invariant is accounts * 1000.")
	w.accounts = max(accounts, minAccounts)

	w.retries, _ = d.Param.Int("retry-attempts", defaultRetries, "Maximum transaction attempts.")
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

	w.move = w.sql.Require("workload_tx_transfer", "move_balance")
	w.total = w.sql.Require("verify", "total_balance")

	// The witness' own queries exist only where the engine can carry it.
	if w.tokenWitness() {
		w.read = w.sql.Require("workload_tx_transfer", "read_account")
		w.tokenOnly = w.sql.Require("workload_tx_transfer", "read_token")
		w.rotate = w.sql.Require("workload_tx_transfer", "rotate_token")
		w.tokens = w.sql.Require("verify", "token_stats")
	}

	for worker := range w.randoms {
		//nolint:gosec // benchmark data generator, not security randomness.
		w.randoms[worker] = rand.New(rand.NewPCG(uint64(worker)+1, rngSeed))
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

// The token is the second witness of the transfer: it is UNIQUE, so the swap cannot be
// half-applied quietly, and it is moved *conditionally* on the value that was read,
// which turns a concurrent transfer into a detected conflict rather than a silent
// overwrite.
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
		if !w.tokenWitness() {
			return w.moveBalances(ctx, tx, from, to, amount)
		}

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

// moveBalances is the reduced transfer: two relative updates, because the token swap
// needs either a transaction or a unique constraint to mean anything.
func (w *workload) moveBalances(ctx context.Context, tx *bench.Tx, from, to, amount int64) error {
	for _, move := range balanceMoves(from, to, amount) {
		if err := tx.Exec(ctx, w.move.Text, map[string]any{"id": move.id, "delta": move.delta}); err != nil {
			return fmt.Errorf("move_balance: %w", err)
		}
	}

	return nil
}

// tokenWitness reports whether this driver can carry the UNIQUE token, which turns a
// half-applied swap into a state the balances alone cannot show — a row parked in the
// sentinel, or two rows holding one token.
//
// Picodata cannot: unique indexes are only allowed on sharding-key columns, and its
// single-bucket transactions cannot make a two-row swap atomic. There the run checks
// the balance only, and the report data says so.
func (w *workload) tokenWitness() bool { return w.driverType != bench.DriverPicodata }

// retryOptions makes a transfer retryable for the two failures that classify as
// unknown, on which the default policy fails closed: the conflict this workload
// detects itself, and the fault injector's error.
//
// Retrying is safe because the transaction is the unit of work — a failed attempt
// leaves nothing behind, and even a repeat of a transfer that did commit is
// indistinguishable from two transfers, since it moves a fixed amount and conserves
// the total.
func (w *workload) retryOptions() bench.RetryOptions {
	actions := bench.DefaultErrorActions()
	actions[driver.ErrorKindUnknown] = bench.ErrorActionRetry

	return bench.RetryOptions{
		MaxAttempts: w.retries,
		Idempotent:  true,
		Actions:     actions,
	}
}

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

// swapMoves is the exchange between two accounts as three conditional writes.
//
// Three rather than two because the UNIQUE constraint is immediate and both rows would
// otherwise want a token at once: the first parks its token in a sentinel — the
// negative of its own id, outside 1..accounts and unique to that row — and picks the
// other's up last.
func swapMoves(from, to, fromToken, toToken int64) []tokenMove {
	sentinel := -from

	return []tokenMove{
		{id: from, token: sentinel, expected: fromToken},
		{id: to, token: fromToken, expected: toToken},
		{id: from, token: toToken, expected: sentinel},
	}
}

// balanceMoves is the money side: equal and opposite, so the total is untouched.
func balanceMoves(from, to, amount int64) []balanceMove {
	return []balanceMove{{id: from, delta: -amount}, {id: to, delta: amount}}
}

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

// rotateToken writes a token conditionally on the value read, then confirms it.
//
// Exec exposes no row count, so a conditional update that matched nothing looks like
// one that matched; the confirmation read is what tells them apart. A read sees the
// transaction's own writes, so a different token means a concurrent transfer won — a
// conflict for the retry policy, not a failure of the run.
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

// verifyAttempts and verifyDelay bound how long the check waits for a server that is
// not answering yet: the crash variant leaves the cluster reinitializing, measured at
// 0.3-3.8 s, so a check that gave up on the first refusal could never report on the
// invariants after a crash.
const (
	verifyAttempts = 12
	verifyDelay    = 500 * time.Millisecond
)

func (w *workload) verify(ctx context.Context, b *bench.Bench) error {
	return retryCheck(ctx, verifyAttempts, verifyDelay, func() error {
		return w.checkInvariants(ctx, b)
	})
}

// retryCheck runs check until it succeeds or the attempts are spent. A genuine
// violation is not swallowed: it is reported after the wait, unchanged.
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

// checkInvariants asserts the money is conserved and the tokens are still a
// permutation. Both are conservation properties, so a whole transfer that is lost or
// applied twice leaves each of them unchanged: neither invariant proves that every
// intended transfer happened exactly once. What they catch is a partial one.
func (w *workload) checkInvariants(ctx context.Context, b *bench.Bench) error {
	total, err := b.QueryValue[int64](ctx, w.total.Text, nil)
	if err != nil {
		return fmt.Errorf("verify total: %w", err)
	}

	if want := w.accounts * initialBalance; total != want {
		return fmt.Errorf("%w is %d, want %d", errBalanceMismatch, total, want)
	}

	// The weaker guarantee has to be visible where a run's output is read, not only here.
	if !w.tokenWitness() {
		b.AddReportData("token_witness",
			"unavailable on this driver: unique indexes are limited to sharding-key "+
				"columns, and a two-row transfer is cross-shard so it cannot be atomic — "+
				"the total balance is checked; a half-applied transfer leaves no trace")

		return nil
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

	// No token repeats and they span exactly 1..accounts: with the row count that is the
	// permutation, and a stronger witness than a distinct count — a duplicate paired with
	// a loss still shows up as a repeat.
	if rows != w.accounts || repeated != 0 || lowest != 1 || highest != w.accounts {
		return fmt.Errorf(
			"%w 1..%d (rows %d, repeated %d, min %d, max %d)",
			errTokenPermutation, w.accounts, rows, repeated, lowest, highest,
		)
	}

	return nil
}
