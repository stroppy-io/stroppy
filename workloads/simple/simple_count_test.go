package simple

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestSimpleNoopIterations(t *testing.T) {
	result, err := testkit.Run(t.Context(), Test, bench.RunOptions{
		Params: bench.ParamInputs{CLI: map[string]string{
			"iterations": "5",
			"vus":        "2",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, report.StatusCompleted, result.Status)
	require.NotNil(t, result.Metrics["iterations_total"].Total)
	require.InDelta(t, 5, *result.Metrics["iterations_total"].Total, 0)

	if failed := result.Metrics["failed_iterations_total"].Total; failed != nil {
		require.Zero(t, *failed)
	}

	require.NotNil(t, result.Metrics["run_query_operations_total"].Total)
	require.InDelta(t, 5*4+3, *result.Metrics["run_query_operations_total"].Total, 0)
}

func TestSimpleRowCountValidation(t *testing.T) {
	for _, count := range []int64{demoRows, 1} {
		t.Run(strconv.FormatInt(count, 10), func(t *testing.T) {
			recorder := &record.Recorder{}
			recorder.Reply("SELECT COUNT(*) FROM stroppy_demo", record.Response{
				Columns: []string{"count"}, Rows: [][]any{{count}},
			})

			for range 3 {
				recorder.Reply("SELECT label FROM stroppy_demo WHERE id = :id", record.Response{
					Columns: []string{"label"}, Rows: [][]any{{"demo"}},
				})
			}

			result, err := testkit.Record(t.Context(), Test, recorder, bench.RunOptions{
				Steps:  []string{"workload"},
				Params: bench.ParamInputs{CLI: map[string]string{"iterations": "1"}},
			})
			require.NoError(t, err)

			if count == demoRows {
				require.Equal(t, report.StatusCompleted, result.Status)
				require.Len(t, recorder.Operations(), 4)
			} else {
				require.NotNil(t, result.Metrics["failed_iterations_total"].Total)
				require.InDelta(t, 1, *result.Metrics["failed_iterations_total"].Total, 0)
				require.Len(t, recorder.Operations(), 1)
			}
		})
	}
}

func TestUnsignedRowCount(t *testing.T) {
	for _, tc := range []struct {
		value uint64
		want  int
	}{{100, 100}, {0, 0}, {math.MaxUint64, -1}} {
		if got := toInt(tc.value); got != tc.want {
			t.Fatalf("count %d = %d, want %d", tc.value, got, tc.want)
		}
	}
}
