package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/common"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
)

func TestNamedDriverInsertionFallback(t *testing.T) {
	for _, method := range []InsertStrategy{0, InsertPlainBulk} {
		t.Run(method.String(), func(t *testing.T) {
			test := Test{Name: "named-load", Define: func(d *Def) error {
				ref := d.Drivers.Declare("secondary", DriverConfig{Kind: DriverNoop, DefaultInsertMethod: method})
				d.Execution.Step("load", func(ctx context.Context, b *Bench) error {
					want := method
					if want == 0 {
						want = InsertNative
					}

					require.Equal(t, want.String(), b.cfg.DefaultInsertMethod)

					result, err := b.Insert(ctx, "rows", validInsertSource())
					if err == nil {
						require.Equal(t, int64(1), result.Rows)
					}

					return err
				}, Use(ref))

				return d.Execution.Err()
			}}
			_, err := RunTest(t.Context(), test, noopRunOptions())
			require.NoError(t, err)
		})
	}
}

func TestLogicalOperationNestingAcrossDatabaseFacades(t *testing.T) {
	for _, name := range []string{"default", "secondary"} {
		t.Run(name, func(t *testing.T) {
			test := Test{Name: "facade-transaction", Define: func(d *Def) error {
				ref := d.Drivers.Declare(name, DriverConfig{Kind: DriverNoop})
				d.Execution.Step("work", func(ctx context.Context, b *Bench) error {
					return b.LogicalOperation(func() error {
						return b.Database(ref).Transaction(ctx, TransactionOptions{},
							func(ctx context.Context, tx *Tx) error { return tx.Exec(ctx, "SELECT 1", nil) })
					})
				}, SharedIterations(2, 7))

				return d.Execution.Err()
			}}
			result, err := RunTest(t.Context(), test, noopRunOptions())
			require.NoError(t, err)
			require.InDelta(t, 7, *result.Metrics["successful_transactions_total"].Total, 0)
		})
	}
}

func TestFrameworkMetricNamesFailBeforeActions(t *testing.T) {
	for name := range frameworkMetricNames {
		t.Run(name, func(t *testing.T) {
			var called atomic.Bool

			test := Test{Name: "reserved-metric", Define: func(d *Def) error {
				d.Execution.Step("effect", func(context.Context, *Bench) error {
					called.Store(true)

					return nil
				})
				d.Metrics.Counter(name)

				return d.Execution.Err()
			}}
			_, err := RunTest(t.Context(), test, noopRunOptions())

			var validation *ValidationError
			require.ErrorAs(t, err, &validation)
			require.False(t, called.Load())
		})
	}
}

func TestRateMetricNamesCannotOverlap(t *testing.T) {
	for _, rateFirst := range []bool{false, true} {
		test := Test{Name: "rate-collision", Define: func(d *Def) error {
			if rateFirst {
				d.Metrics.Rate("answers")
			}

			d.Metrics.Counter("answers_events_total")

			if !rateFirst {
				d.Metrics.Rate("answers")
			}

			return nil
		}}
		_, err := DescribeTest(test)

		var validation *ValidationError
		require.ErrorAs(t, err, &validation)
	}
}

func TestRuntimeMetricRegistrationFailureReturnsValidation(t *testing.T) {
	root := newRuntimeTestRoot(t)
	_, err := root.registry.NewMetric("iterations_total", Counter)
	require.NoError(t, err)

	var value any

	func() {
		defer func() { value = recover() }()

		root.txMetrics.ensureRegistered(&VU{root: root, ctx: t.Context()})
	}()
	require.IsType(t, &ValidationError{}, value)
}

func TestLoggerSerializesSharedWriter(t *testing.T) {
	var output bytes.Buffer

	test := Test{Name: "logging", Define: func(d *Def) error {
		d.Execution.Step("work", func(_ context.Context, b *Bench) error {
			b.Log.With("worker", b.Worker()).Info("record", "iteration", b.Iteration())

			return nil
		}, SharedIterations(4, 100))

		return d.Execution.Err()
	}}
	options := noopRunOptions()
	options.Logger = NewLogger(&output)
	_, err := RunTest(t.Context(), test, options)
	require.NoError(t, err)

	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	require.Len(t, lines, 100)

	for _, line := range lines {
		require.True(t, json.Valid(line), string(line))
	}
}

type panicInsertDriver struct {
	recordingDriver
	closed  atomic.Bool
	tracker *insertprogress.Tracker
}

func (d *panicInsertDriver) Insert(ctx context.Context, request *driver.InsertRequest) (*stats.Query, error) {
	d.tracker = insertprogress.FromContext(ctx)
	_, err := common.RunParallelBatch(ctx, request.Source, request.Workers, 1,
		func(_ context.Context, _ common.Chunk, cursor gen.Cursor) error {
			_, err := cursor.Next()

			return err
		})

	return nil, err
}

func (d *panicInsertDriver) Teardown(ctx context.Context) error {
	d.closed.Store(ctx.Err() == nil)

	return nil
}

func TestInsertPanicJoinsAndCleansBeforePropagation(t *testing.T) {
	backend := &panicInsertDriver{}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	test := Test{Name: "load-panic", Define: func(d *Def) error {
		d.Execution.Step("load", func(ctx context.Context, b *Bench) error {
			b.drv = backend
			b.execution.databaseMu.Lock()
			b.execution.databases[""] = &databaseSlot{drv: backend, cfg: b.cfg}
			b.execution.databaseMu.Unlock()

			source := gen.FromRows(1, func(uint64) (struct{ ID int64 }, error) { panic("row panic") })
			_, err := b.Insert(ctx, "rows", source)

			return err
		})

		return d.Execution.Err()
	}}

	require.PanicsWithValue(t, "row panic", func() { _, _ = RunTest(ctx, test, noopRunOptions()) })
	require.True(t, backend.closed.Load())
	require.NotNil(t, backend.tracker)
	require.Equal(t, insertprogress.EventFailed, backend.tracker.Finish(context.Canceled).Event)
}
