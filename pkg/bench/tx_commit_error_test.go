package bench

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/stats"
)

type commitFailTx struct {
	driver.Tx

	commitErr error
	rollbacks int
	commits   int
	applied   int
	rejected  bool
}

func (tx *commitFailTx) Commit(context.Context) error {
	tx.commits++
	if !tx.rejected || tx.commits > 1 {
		tx.applied++
	}

	if tx.commits == 1 {
		return tx.commitErr
	}

	return nil
}

func (tx *commitFailTx) Rollback(context.Context) error {
	tx.rollbacks++

	return nil
}

type commitFailDriver struct {
	tx    *commitFailTx
	facts driver.ErrorFacts
}

func (d *commitFailDriver) Insert(context.Context, *driver.InsertRequest) (*stats.Query, error) {
	return nil, errors.New("unexpected Insert")
}

func (d *commitFailDriver) RunQuery(context.Context, string, map[string]any) (*driver.QueryResult, error) {
	return nil, errors.New("unexpected RunQuery")
}

func (d *commitFailDriver) Begin(context.Context, config.TxIsolationLevel) (driver.Tx, error) {
	return d.tx, nil
}

func (d *commitFailDriver) ClassifyError(error) driver.ErrorFacts { return d.facts }

func (d *commitFailDriver) Teardown(context.Context) error { return nil }

func TestCommitRetrySafety(t *testing.T) {
	for _, path := range []string{"managed", "manual", "transaction"} {
		t.Run(path, func(t *testing.T) {
			for _, test := range []struct {
				name       string
				kind       driver.ErrorKind
				idempotent bool
				rejected   bool
				wantRetry  bool
			}{
				{name: "applied ack lost", kind: driver.ErrorKindTransient},
				{name: "unknown retry override", kind: driver.ErrorKindUnknown},
				{name: "lock timeout", kind: driver.ErrorKindLockTimeout},
				{name: "serialization rejected", kind: driver.ErrorKindSerialization, rejected: true, wantRetry: true},
				{name: "deadlock rejected", kind: driver.ErrorKindDeadlock, rejected: true, wantRetry: true},
				{name: "idempotent", kind: driver.ErrorKindTransient, idempotent: true, wantRetry: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					root := newRuntimeTestRoot(t)
					commitErr := errors.New("commit reply lost")
					tx := &commitFailTx{commitErr: commitErr, rejected: test.rejected}
					drv := &commitFailDriver{tx: tx, facts: driver.ErrorFacts{Kind: test.kind}}
					b := &Bench{
						root: root,
						vu:   &VU{root: root, vuid: 1, ctx: context.Background()},
						lg:   zap.NewNop(),
						drv:  drv,
						cfg:  &config.DriverConfig{DriverType: config.DriverTypePostgres},
					}
					bodies := 0
					opts := TxRetryPolicyOptions{
						MaxAttempts: 3,
						Idempotent:  test.idempotent,
						Actions: ErrorActionMap{ //nolint:exhaustive // exercise retry overrides without replaying an ambiguous commit
							driver.ErrorKindUnknown: ErrorActionRetry,
						},
					}
					ctx := context.Background()
					body := func(*Tx) error {
						bodies++

						return ctx.Err()
					}
					err := runCommitRetryPath(ctx, b, path, opts, body)

					wantAttempts, wantApplied := 1, 1

					if test.wantRetry {
						require.NoError(t, err)

						wantAttempts = 2

						if !test.rejected {
							wantApplied = 2
						}
					} else {
						require.ErrorIs(t, err, commitErr)
					}

					require.Equal(t, wantAttempts, bodies, "transaction body executions")
					require.Equal(t, wantAttempts, tx.commits)
					require.Equal(t, wantApplied, tx.applied, "applied transactions")
					require.Zero(t, tx.rollbacks, "a commit attempt finalizes the handle")
					require.Equal(t, uint64(wantAttempts-1), root.errorReporter.snapshot().retryAttempts)
				})
			}
		})
	}
}

func runCommitRetryPath(
	ctx context.Context,
	b *Bench,
	path string,
	opts TxRetryPolicyOptions,
	body func(*Tx) error,
) error {
	if path == "transaction" {
		return b.Transaction(ctx, TransactionOptions{Isolation: IsoReadCommitted, Retry: opts},
			func(_ context.Context, tx *Tx) error { return body(tx) })
	}

	return Retry0(ctx, b.TxRetryPolicy(opts), func() error {
		if path == "managed" {
			return b.BeginTx(ctx, BeginOpts{Isolation: IsoReadCommitted}, body)
		}

		tx, err := b.Begin(ctx, BeginOpts{Isolation: IsoReadCommitted})
		if err != nil {
			return err
		}

		if err := body(tx); err != nil {
			return errors.Join(err, tx.Rollback(ctx))
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("new order: %w", err)
		}

		return nil
	})
}

// TestRejectedCommitStillEndsTheTransaction pins what a failed COMMIT must do
// beyond returning its error.
//
// The transaction is over either way, so it has to be recorded as ended. Before
// this, the error returned before the end was recorded, which had two effects a
// caller could see: the deferred cleanup in BeginTx rolled back a transaction that
// was already closed — surfacing to the client as a spurious "tx is closed" beside
// the real error — and the failure never reached the tx metrics, so `tx_errors_total`
// under-reported exactly the runs worth measuring.
func TestRejectedCommitStillEndsTheTransaction(t *testing.T) {
	rootState := newRuntimeTestRoot(t)

	commitErr := errors.New("commit rejected by the server")
	tx := &commitFailTx{commitErr: commitErr}
	drv := &commitFailDriver{tx: tx}

	b := &Bench{
		root: rootState,
		vu:   &VU{root: rootState, vuid: 1, ctx: context.Background()},
		lg:   zap.NewNop(),
		drv:  drv,
		cfg: &config.DriverConfig{
			DriverType: config.DriverTypePostgres,
		},
	}

	var handle *Tx

	err := b.BeginTx(context.Background(), BeginOpts{Name: "transfer"}, func(inner *Tx) error {
		handle = inner

		return nil
	})

	require.ErrorIs(t, err, commitErr, "the commit error must reach the caller unchanged")
	require.NotNil(t, handle)

	if !handle.done {
		t.Error("a rejected COMMIT left the transaction open: the deferred cleanup will " +
			"try to roll back a closed transaction, and the metrics will not record the failure")
	}

	if tx.rollbacks != 0 {
		t.Errorf("the transaction was rolled back %d time(s) after its commit had already failed", tx.rollbacks)
	}
}
