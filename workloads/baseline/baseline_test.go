package baseline_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/config"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
)

func TestBaselineDescribeSchema(t *testing.T) {
	description, err := bench.Describe("baseline")
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}

	var names []string

	for _, param := range description.Params {
		if param.Scope == bench.ParamScopeWorkload {
			names = append(names, param.Name)
		}
	}

	for _, want := range []string{"rows", "load-workers", "tx-isolation"} {
		if !slices.Contains(names, want) {
			t.Fatalf("workload params = %v, want %q present", names, want)
		}
	}
}

// TestBaselineRunsCleanUnderNoop pins the contract the baseline command relies
// on: every iteration completes against the noop driver (no result validation
// to trip on stub rows), and the load reports exactly the requested row count.
func TestBaselineRunsCleanUnderNoop(t *testing.T) {
	var captured metricdata.ResourceMetrics

	calls := 0

	err := bench.Run(
		context.Background(),
		"baseline",
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		bench.ParamInputs{CLI: map[string]string{
			"rows":         "1000",
			"load-workers": "2",
			"iterations":   "25",
			"vus":          "2",
		}},
		nil,
		nil,
		zap.NewNop(),
		&bench.MetricsConfig{
			Quiet: true,
			OnSummary: func(data metricdata.ResourceMetrics) {
				captured = data
				calls++
			},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if calls != 1 {
		t.Fatalf("OnSummary calls = %d, want 1", calls)
	}

	values := summaryValues(captured)

	if got := values["iterations_total"]; got != 25 {
		t.Fatalf("iterations_total = %v, want 25", got)
	}

	if got, present := values["failed_iterations_total"]; present && got != 0 {
		t.Fatalf("failed_iterations_total = %v, want 0", got)
	}

	if got := values["insert_rows_total"]; got != 1000 {
		t.Fatalf("insert_rows_total = %v, want 1000", got)
	}

	if got := values["tx_commits_total"]; got != 25 {
		t.Fatalf("tx_commits_total = %v, want 25", got)
	}
}

func summaryValues(data metricdata.ResourceMetrics) map[string]float64 {
	values := map[string]float64{}

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			name := strings.TrimPrefix(metric.Name, "stroppy_")

			if sum, ok := metric.Data.(metricdata.Sum[float64]); ok {
				for _, point := range sum.DataPoints {
					values[name] += point.Value
				}
			}
		}
	}

	return values
}
