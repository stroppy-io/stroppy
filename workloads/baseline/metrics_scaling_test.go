package baseline_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// BenchmarkMetricsScaling exercises the real transaction and metrics paths without database I/O.
func BenchmarkMetricsScaling(b *testing.B) {
	for _, workers := range []int{1, 8, 18, 24} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			b.ReportAllocs()

			_, err := bench.RunCatalog(b.Context(), bench.RegisteredCatalog(), "baseline", bench.RunOptions{
				Drivers: map[string]bench.DriverConfig{"": {Kind: bench.DriverNoop}},
				Params: bench.ParamInputs{CLI: map[string]string{
					"executor":   "shared-iterations",
					"iterations": strconv.Itoa(b.N),
					"vus":        strconv.Itoa(workers),
				}},
				Steps:   []string{"workload"},
				Metrics: &bench.MetricsConfig{Quiet: true},
			})
			if err != nil {
				b.Fatal(err)
			}
		})
	}
}
