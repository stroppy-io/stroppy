package testkit_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestRecordTransactionsReadsAndLoads(t *testing.T) {
	recorder := &record.Recorder{SampleRows: 2}
	recorder.Reply("SELECT :id", record.Response{Columns: []string{"value"}, Rows: [][]any{{int64(7)}}})

	test := bench.Test{Name: "record-test", Define: func(d *bench.Def) error {
		second := d.Drivers.Declare("secondary", bench.DriverConfig{Kind: bench.DriverPostgres})
		d.Execution.Step("load", func(ctx context.Context, b *bench.Bench) error {
			source := gen.FromRows(3, func(index uint64) (struct{ ID int64 }, error) {
				return struct{ ID int64 }{int64(index)}, nil
			})
			_, err := b.Insert(ctx, "items", source, bench.LoadWorkers(2))

			return err
		})
		d.Execution.Step("work", func(ctx context.Context, b *bench.Bench) error {
			return b.Transaction(ctx, bench.TransactionOptions{}, func(ctx context.Context, tx *bench.Tx) error {
				value, err := tx.QueryValue[int64](ctx, "SELECT :id", map[string]any{"id": int64(1)})
				if err != nil {
					return err
				}

				if value != 7 {
					return errors.New("unexpected canned value")
				}

				return b.Database(second).Exec(ctx, "UPDATE items", nil)
			})
		}, bench.SharedIterations(1, 1))

		return d.Execution.Err()
	}}
	result, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	require.NoError(t, err)
	require.Equal(t, report.StatusCompleted, result.Status)

	operations := recorder.Operations()
	require.Len(t, operations, 5)
	require.Equal(t, "insert", operations[0].Kind)
	require.Equal(t, int64(3), operations[0].Rows)
	require.Len(t, operations[0].Sample, 2)
	require.Equal(t, "begin", operations[1].Kind)
	require.Equal(t, "query", operations[2].Kind)
	require.Equal(t, "secondary", operations[3].Scope.Database)
	require.Equal(t, "commit", operations[4].Kind)

	var first, second bytes.Buffer
	require.NoError(t, recorder.WriteTo(&first))
	require.NoError(t, recorder.WriteTo(&second))
	require.Equal(t, first.String(), second.String())
}

func TestNoopCancellationRunsCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cleaned := false
	test := bench.Test{Name: "cancel-test", Define: func(d *bench.Def) error {
		d.Execution.Step("work", func(ctx context.Context, _ *bench.Bench) error {
			cancel()

			return ctx.Err()
		})
		d.Execution.Step("cleanup", func(ctx context.Context, _ *bench.Bench) error {
			cleaned = ctx.Err() == nil

			return nil
		}, bench.Always(time.Second))

		return d.Execution.Err()
	}}
	_, err := testkit.Run(ctx, test, bench.RunOptions{})
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, cleaned)
}
