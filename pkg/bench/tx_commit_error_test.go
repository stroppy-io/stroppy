package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/stats"
)

// commitFailTx is a transaction whose COMMIT is rejected, which is the case a
// fault-injecting workload produces on purpose and any overtaxed server produces
// by accident.
type commitFailTx struct {
	driver.Tx

	commitErr error
	rollbacks int
}

func (tx *commitFailTx) Commit(context.Context) error { return tx.commitErr }

func (tx *commitFailTx) Rollback(context.Context) error {
	tx.rollbacks++

	return nil
}

type commitFailDriver struct {
	tx *commitFailTx
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

func (d *commitFailDriver) ClassifyError(error) driver.ErrorFacts { return driver.ErrorFacts{} }

func (d *commitFailDriver) Teardown(context.Context) error { return nil }

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
