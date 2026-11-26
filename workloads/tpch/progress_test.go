package tpch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
)

func TestProgressCountsAvailableQueriesAndRepetitions(t *testing.T) {
	test := bench.Test{Name: "query-progress", Define: func(d *bench.Def) error {
		w := &workload{sql: bench.ParseSQL("--+ q1\n--= body\nSELECT 1;\n--+ q3\n--= body\nSELECT 3;")}
		w.m = w.initMetrics(&d.Metrics)
		d.Execution.Step("workload", func(ctx context.Context, b *bench.Bench) error {
			return w.runQueries(ctx, b)
		}, bench.SharedIterations(1, 3))

		return d.Execution.Err()
	}}
	run, err := testkit.Run(t.Context(), test, bench.RunOptions{})
	require.NoError(t, err)

	for _, source := range run.Progress.Sources {
		if source.Unit == "queries" {
			require.InDelta(t, 6.0, source.Current, 0)
			require.NotNil(t, source.Total)
			require.InDelta(t, 6.0, *source.Total, 0)

			return
		}
	}

	t.Fatal("query progress missing")
}
