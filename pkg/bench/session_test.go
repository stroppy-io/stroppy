package bench_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestWorkerPreparedSessionLifecycle(t *testing.T) {
	const workers = 3

	var (
		initialized atomic.Int64
		held        []*bench.Prepared
	)

	recorder := &record.Recorder{}
	test := bench.Test{Name: "prepared", Define: func(d *bench.Def) error {
		other := d.Drivers.Declare("other", bench.DriverConfig{})
		held = make([]*bench.Prepared, workers)
		d.Execution.WorkerStep("work", func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
			s, err := b.Session(ctx)
			if err != nil {
				return nil, err
			}

			same, err := b.Session(ctx)
			if err != nil {
				return nil, err
			}

			if same != s {
				return nil, errors.New("worker session was not reused")
			}

			named, err := b.Database(other).Session(ctx)
			if err != nil {
				return nil, err
			}

			namedAgain, err := b.Database(other).Session(ctx)
			if err != nil {
				return nil, err
			}

			if namedAgain != named || named == s {
				return nil, errors.New("named session identity mismatch")
			}

			statement, err := s.Prepare(ctx, bench.QueryHandle{Name: "increment", Text: "UPDATE counters SET n = n + :delta"})
			if err != nil {
				return nil, err
			}

			held[b.Worker()] = statement

			initialized.Add(1)

			return func(ctx context.Context, b *bench.Bench) error {
				if initialized.Load() != workers {
					return bench.Fatal(errors.New("action started before initialization barrier"))
				}

				return b.LogicalOperation(func() error {
					return s.BeginTx(ctx, bench.BeginOpts{Name: "increment", Isolation: "read_committed"}, func(tx *bench.Tx) error {
						return tx.ExecPrepared(ctx, statement, map[string]any{"delta": int64(1)})
					})
				})
			}, nil
		}, bench.SharedIterations(workers, 12))

		return d.Execution.Err()
	}}
	result, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	require.NoError(t, err)
	require.Positive(t, result.Executions["work"].StartupSeconds)
	require.InDelta(t, 12., *result.Metrics["successful_transactions_total"].Total, 0)
	require.InDelta(t, 12., *result.Metrics["run_query_operations_total"].Total, 0)

	counts := map[string]int{}
	for _, op := range recorder.Operations() {
		counts[op.Kind]++
		if op.Kind == "query" {
			require.NotZero(t, op.Session)
			require.NotZero(t, op.Statement)
			require.NotZero(t, op.Transaction)
			require.Equal(t, "increment", op.QueryName)
		}
	}

	require.Equal(t, workers*2, counts["session_open"])
	require.Equal(t, workers*2, counts["session_close"])
	require.Equal(t, workers, counts["prepare"])
	require.Equal(t, workers, counts["statement_close"])
	require.Equal(t, 12, counts["query"])
	require.Equal(t, 12, counts["commit"])

	for _, statement := range held {
		require.ErrorIs(t, statement.Exec(t.Context(), nil), bench.ErrSessionClosed)
	}
}

func TestWorkerInitializationFailureClosesPartialSessions(t *testing.T) {
	failure := errors.New("initialization failed")
	recorder := &record.Recorder{}

	var actions atomic.Int64

	test := bench.Test{Name: "partial", Define: func(d *bench.Def) error {
		d.Execution.WorkerStep("work", func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
			s, err := b.Session(ctx)
			if err != nil {
				return nil, err
			}

			if _, err := s.Prepare(ctx, bench.QueryHandle{Name: "one", Text: "SELECT 1"}); err != nil {
				return nil, err
			}

			return nil, failure
		}, bench.SharedIterations(1, 10))

		return d.Execution.Err()
	}}
	_, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	require.ErrorIs(t, err, failure)
	require.Zero(t, actions.Load())

	operations := recorder.Operations()

	kinds := make([]string, 0, len(operations))
	for _, op := range operations {
		kinds = append(kinds, op.Kind)
	}

	require.Equal(t, []string{"session_open", "prepare", "statement_close", "session_close"}, kinds)
}

//nolint:cyclop // each read form and affinity rejection is checked against one session.
func TestPreparedReadFormsAndTransactionAffinity(t *testing.T) {
	recorder := &record.Recorder{}

	const sql = "SELECT :value AS value"
	for range 7 {
		recorder.Reply(sql, record.Response{Columns: []string{"value"}, Rows: [][]any{{int64(42)}}})
	}

	test := bench.Test{Name: "reads", Define: func(d *bench.Def) error {
		other := d.Drivers.Declare("other", bench.DriverConfig{})
		d.Execution.WorkerStep("work", func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
			s, err := b.Session(ctx)
			if err != nil {
				return nil, err
			}

			p, err := s.Prepare(ctx, bench.QueryHandle{Name: "value", Text: sql})
			if err != nil {
				return nil, err
			}

			return func(ctx context.Context, b *bench.Bench) error {
				args := map[string]any{"value": int64(42)}

				value, err := p.QueryValue[int64](ctx, args)
				if err != nil || value != 42 {
					return fmt.Errorf("scalar: %d: %w", value, err)
				}

				values, err := p.QueryValues[int64](ctx, args)
				if err != nil || len(values) != 1 || values[0] != 42 {
					return fmt.Errorf("values: %v: %w", values, err)
				}

				row, err := p.QueryRow(ctx, args)
				if err != nil {
					return err
				}

				if actual, readErr := row.Int64("value"); readErr != nil || actual != 42 {
					return errors.New("owned row mismatch")
				}

				if _, err := p.RawRow(ctx, args); err != nil {
					return err
				}

				if _, err := p.RawRows(ctx, args); err != nil {
					return err
				}

				rows, err := p.Query(ctx, args)
				if err != nil {
					return err
				}

				if !rows.Next() {
					return errors.New("missing streaming row")
				}

				if err := rows.Close(); err != nil {
					return err
				}

				named, err := b.Database(other).Session(ctx)
				if err != nil {
					return err
				}

				wrong, err := named.Prepare(ctx, bench.QueryHandle{Name: "wrong", Text: "SELECT 2"})
				if err != nil {
					return err
				}

				return s.BeginTx(ctx, bench.BeginOpts{Isolation: "read_committed"}, func(tx *bench.Tx) error {
					if err := tx.ExecPrepared(ctx, wrong, nil); !errors.Is(err, bench.ErrSessionMismatch) {
						return fmt.Errorf("wrong affinity: %w", err)
					}

					if err := p.Exec(ctx, args); !errors.Is(err, bench.ErrSessionTransaction) {
						return fmt.Errorf("direct execution during transaction: %w", err)
					}

					_, err := tx.QueryRowPrepared(ctx, p, args)

					return err
				})
			}, nil
		}, bench.SharedIterations(1, 1))

		return d.Execution.Err()
	}}
	_, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	require.NoError(t, err)
}

func TestRecordingSessionRejectsOverlappingCursors(t *testing.T) {
	recorder := &record.Recorder{}
	test := bench.Test{Name: "cursor", Define: func(d *bench.Def) error {
		d.Execution.Step("work", func(ctx context.Context, b *bench.Bench) error {
			s, err := b.Session(ctx)
			if err != nil {
				return err
			}

			p, err := s.Prepare(ctx, bench.QueryHandle{Name: "one", Text: "SELECT 1"})
			if err != nil {
				return err
			}

			rows, err := p.Query(ctx, nil)
			if err != nil {
				return err
			}

			if err := p.Exec(ctx, nil); !errors.Is(err, bench.ErrSessionBusy) {
				return fmt.Errorf("overlapping cursor: %w", err)
			}

			if err := rows.Close(); err != nil {
				return err
			}

			return p.Exec(ctx, nil)
		})

		return d.Execution.Err()
	}}
	_, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	require.NoError(t, err)
}

func TestWorkerCancellationRollsBackAndCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	recorder := &record.Recorder{}
	test := bench.Test{Name: "cancel", Define: func(d *bench.Def) error {
		d.Execution.WorkerStep("work", func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
			s, err := b.Session(ctx)
			if err != nil {
				return nil, err
			}

			p, err := s.Prepare(ctx, bench.QueryHandle{Name: "one", Text: "SELECT 1"})
			if err != nil {
				return nil, err
			}

			return func(ctx context.Context, _ *bench.Bench) error {
				return s.BeginTx(ctx, bench.BeginOpts{Isolation: "read_committed"}, func(tx *bench.Tx) error {
					if _, err := tx.QueryPrepared(ctx, p, nil); err != nil {
						return err
					}

					cancel()

					return ctx.Err()
				})
			}, nil
		}, bench.ConstantWorkers(1, time.Second, bench.DrainHalfMinuteTimeout))

		return d.Execution.Err()
	}}
	_, err := testkit.Record(ctx, test, recorder, bench.RunOptions{})
	require.ErrorIs(t, err, context.Canceled)

	kinds := map[string]int{}
	for _, op := range recorder.Operations() {
		kinds[op.Kind]++
	}

	require.Equal(t, 1, kinds["rollback"])
	require.Equal(t, 1, kinds["statement_close"])
	require.Equal(t, 1, kinds["session_close"])
}

func TestWorkerPanicClosesPreparedSessions(t *testing.T) {
	for _, initializePanic := range []bool{true, false} {
		t.Run(strconv.FormatBool(initializePanic), func(t *testing.T) {
			recorder := &record.Recorder{}
			test := bench.Test{Name: "panic", Define: func(d *bench.Def) error {
				d.Execution.WorkerStep("work", func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
					s, err := b.Session(ctx)
					if err != nil {
						return nil, err
					}

					if _, err := s.Prepare(ctx, bench.QueryHandle{Name: "one", Text: "SELECT 1"}); err != nil {
						return nil, err
					}

					if initializePanic {
						panic("worker panic")
					}

					return func(context.Context, *bench.Bench) error { panic("worker panic") }, nil
				}, bench.SharedIterations(1, 1))

				return d.Execution.Err()
			}}

			require.PanicsWithValue(t, "worker panic", func() {
				_, _ = testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
			})

			counts := map[string]int{}
			for _, op := range recorder.Operations() {
				counts[op.Kind]++
			}

			require.Equal(t, 1, counts["statement_close"])
			require.Equal(t, 1, counts["session_close"])
		})
	}
}

func TestSessionUnsupportedAndWorkerObservation(t *testing.T) {
	var called atomic.Int64

	test := bench.Test{Name: "unsupported", Define: func(d *bench.Def) error {
		d.Execution.WorkerStep("work", func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
			called.Add(1)

			_, err := b.Session(ctx)

			return nil, err
		}, bench.SharedIterations(1, 1))

		return d.Execution.Err()
	}}
	_, err := testkit.Run(t.Context(), test, bench.RunOptions{})
	require.ErrorIs(t, err, bench.ErrSessionUnsupported)
	require.EqualValues(t, 1, called.Load())
}
