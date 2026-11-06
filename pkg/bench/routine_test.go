package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type routineTestDriver struct {
	*commitFailDriver
	outcome error
	calls   int
}

func (d *routineTestDriver) RunRoutine(
	context.Context, config.TxIsolationLevel, string, map[string]any,
) (*driver.QueryResult, error) {
	d.calls++

	return nil, d.outcome
}

func TestRoutineRetryOutcome(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ambiguous bool
		confirmed bool
		kind      driver.ErrorKind
		attempts  int
	}{
		{"server rollback", false, true, driver.ErrorKindSerialization, 3},
		{"lost acknowledgement", true, false, driver.ErrorKindSerialization, 1},
		{"connect failure", false, false, driver.ErrorKindTransient, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newRuntimeTestRoot(t)
			backend := &routineTestDriver{
				commitFailDriver: &commitFailDriver{facts: driver.ErrorFacts{Kind: tc.kind}},
				outcome: &driver.RoutineError{
					Cause: errors.New("routine failure"), Ambiguous: tc.ambiguous, RollbackConfirmed: tc.confirmed,
				},
			}
			b := &Bench{
				root: root, vu: &VU{root: root, vuid: 1, ctx: t.Context()}, lg: zap.NewNop(), drv: backend,
				cfg: &config.DriverConfig{DriverType: config.DriverTypePostgres},
			}
			err := Retry0(t.Context(), b.TxRetryPolicy(TxRetryPolicyOptions{MaxAttempts: 3}), func() error {
				return b.Routine(t.Context(), BeginOpts{Isolation: IsoSerializable, Name: "routine"}, "SELECT routine()", nil)
			})
			require.Error(t, err)
			require.Equal(t, tc.attempts, backend.calls)
		})
	}
}
