package tpcds

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
)

func TestProgressCountsSelectedQueriesPerStream(t *testing.T) {
	test := bench.Test{Name: "stream-progress", Define: func(d *bench.Def) error {
		w := &workload{genStream: -1, querySQL: bench.ParseSQL("--= query_3\nSELECT 3;\n--= query_8\nSELECT 8;")}
		w.metrics = declareQueryMetrics(&d.Metrics, w.querySQL.Names(""))
		d.Execution.Step("workload", w.work, bench.OncePerWorker(2))

		return d.Execution.Err()
	}}
	run, err := testkit.Run(t.Context(), test, bench.RunOptions{})
	require.NoError(t, err)

	workers := make(map[int]bool)

	for _, source := range run.Progress.Sources {
		if source.Unit == "queries" {
			require.Equal(t, "query_8", source.Activity)
			require.InDelta(t, 2.0, source.Current, 0)
			require.NotNil(t, source.Total)
			require.InDelta(t, 2.0, *source.Total, 0)
			workers[source.Worker] = true
		}
	}

	require.Len(t, workers, 2)
}
