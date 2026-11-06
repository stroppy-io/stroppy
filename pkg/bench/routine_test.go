package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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

func TestJoinedRoutineAmbiguityPreventsRetry(t *testing.T) {
	confirmed := &driver.RoutineError{Cause: errors.New("serialization"), RollbackConfirmed: true}

	ambiguous := &driver.RoutineError{Cause: errors.New("reply lost"), Ambiguous: true}
	for _, outcome := range []error{errors.Join(confirmed, ambiguous), errors.Join(ambiguous, confirmed)} {
		classify := func(error) driver.ErrorFacts { return driver.ErrorFacts{Kind: driver.ErrorKindSerialization} }
		policy := newTxRetryPolicy(classify, TxRetryPolicyOptions{MaxAttempts: 3})
		require.False(t, policy.Retries(outcome))
		require.False(t, RoutineRollbackConfirmed(outcome))
	}
}

func TestRoutineOutcomeMetrics(t *testing.T) {
	for _, tc := range []struct {
		name, action string
		outcome      error
		transaction  bool
	}{
		{"commit", "commit", nil, true},
		{"rollback", "rollback", &driver.RoutineError{Cause: errors.New("rejected"), RollbackConfirmed: true}, true},
		{"unknown", "unknown", &driver.RoutineError{Cause: errors.New("lost"), Ambiguous: true}, false},
		{"setup", "setup_error", &driver.RoutineError{Cause: errors.New("connect")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newRuntimeTestRoot(t)
			backend := &routineTestDriver{commitFailDriver: &commitFailDriver{}, outcome: tc.outcome}
			b := &Bench{
				root: root, vu: &VU{root: root, vuid: 1, ctx: t.Context()}, drv: backend,
				cfg: &config.DriverConfig{DriverType: config.DriverTypePostgres},
			}
			_ = b.Routine(t.Context(), BeginOpts{Name: "routine"}, "SELECT routine()", nil)

			var data metricdata.ResourceMetrics
			require.NoError(t, root.manualReader.Collect(t.Context(), &data))

			var transactions float64

			for _, scope := range data.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if metric.Name == root.metricsPrefix+"transactions_total" {
						for _, point := range metric.Data.(metricdata.Sum[float64]).DataPoints {
							transactions += point.Value
						}
					}
				}
			}

			if tc.transaction {
				require.InDelta(t, 1, transactions, 0)
			} else {
				require.Zero(t, transactions)
			}

			histogram := findHistogram(t, data, root.metricsPrefix+"tx_total_duration")
			action, ok := histogram.DataPoints[0].Attributes.Value("tx_action")
			require.True(t, ok)
			require.Equal(t, tc.action, action.AsString())
		})
	}
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
