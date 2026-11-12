package tpcc

import (
	"context"
	"strconv"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
)

func BenchmarkSuccessTPCCProcs(b *testing.B) {
	for _, workers := range []int{1, 4} {
		b.Run(strconv.Itoa(workers), func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				_, err := bench.RunTest(context.Background(), Procs, bench.RunOptions{
					Drivers: map[string]bench.DriverConfig{"": {Kind: bench.DriverNoop}},
					Params: bench.ParamInputs{CLI: map[string]string{
						"vus": strconv.Itoa(workers), "iterations": "1000", "pacing": "false",
					}},
					Steps: []string{"workload"}, Metrics: &bench.MetricsConfig{Quiet: true},
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
