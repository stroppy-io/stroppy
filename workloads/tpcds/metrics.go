package tpcds

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// Fixed buckets keep memory independent of run length. The final boundary covers
// every time.Duration, so hours-long queries cannot fall into an unbounded tail.
var queryDurationBounds = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100,
	250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 120000, 300000, 600000,
	1800000, 3600000, 7200000, 14400000, 28800000, 43200000, 86400000,
	172800000, 604800000, 2592000000, 31536000000, float64(time.Duration(1<<63-1)) / float64(time.Millisecond),
}

type queryMetrics struct {
	duration     *bench.HistogramHandle
	runs         *bench.CounterHandle
	errors       *bench.CounterHandle
	elapsedTotal *bench.CounterHandle
}

func canonicalQueryName(name string) string {
	if strings.HasPrefix(name, "query") && len(name) > len("query") && name[len("query")] != '_' {
		return "query_" + strings.TrimPrefix(name, "query")
	}

	return name
}

func declareQueryMetrics(declarations *bench.MetricDeclarations, names []string) map[string]*queryMetrics {
	unique := make(map[string]bool)

	for i := 1; i <= 99; i++ {
		name := fmt.Sprintf("query_%d", i)
		switch i {
		case 14, 23, 24, 39:
			unique[name+"_a"], unique[name+"_b"] = true, true
		default:
			unique[name] = true
		}
	}

	for _, name := range names {
		unique[canonicalQueryName(name)] = true
	}

	names = make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}

	sort.Strings(names)
	metrics := make(map[string]*queryMetrics, len(names))

	for _, name := range names {
		prefix := "tpcds_" + name
		metrics[name] = &queryMetrics{
			duration: declarations.Histogram(prefix+"_duration", bench.Unit("ms"),
				bench.Bounds(queryDurationBounds...), bench.LabelValues("outcome", "success", "error")),
			runs:         declarations.Counter(prefix + "_runs"),
			errors:       declarations.Counter(prefix + "_errors"),
			elapsedTotal: declarations.Counter(prefix+"_elapsed_total", bench.Unit("ms")),
		}
	}

	return metrics
}

func (metrics *queryMetrics) record(ctx context.Context, elapsed time.Duration, failed bool) {
	milliseconds := float64(elapsed) / float64(time.Millisecond)

	outcome, errors := "success", 0.
	if failed {
		outcome, errors = "error", 1
	}

	metrics.runs.Add(ctx, 1)
	metrics.errors.Add(ctx, errors)
	metrics.elapsedTotal.Add(ctx, milliseconds)
	metrics.duration.Record(ctx, milliseconds, bench.LabelValue("outcome", outcome))
}
