package xk6air

import (
	"context"
	"testing"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/lib"
)

// minimal VU stub: State()==nil makes SetStepTag return right after the
// step-state update, so the benchmark measures exactly the synchronization
// cost of the step-tag path — the code that serialized every VU.
type benchVU struct{}

func (benchVU) Context() context.Context             { return context.Background() }
func (benchVU) Events() common.Events                { return common.Events{} }
func (benchVU) InitEnv() *common.InitEnvironment     { return nil }
func (benchVU) State() *lib.State                    { return nil }
func (benchVU) Runtime() *sobek.Runtime              { return nil }
func (benchVU) RegisterCallback() func(func() error) { return nil }

var _ modules.VU = benchVU{}

// BenchmarkStepTagParallel mirrors the real load: one Instance per VU, all
// flipping their step tag every iteration. With a shared mutex this serializes
// across VUs; with a per-VU field it scales linearly. Run at -cpu=1,8,16 to
// surface contention regressions.
func BenchmarkStepTagParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		i := &Instance{vu: benchVU{}} // one Instance per worker, like k6 per VU
		for pb.Next() {
			i.SetStepTag("load_data")
			i.ClearStepTag("load_data")
		}
	})
}

// BenchmarkTxMetricsSnapshotParallel stresses the metric-snapshot read path
// hit by every tx/query/insert. Snapshots return immutable pointers set once
// at registration, so the read path must stay lock-free.
func BenchmarkTxMetricsSnapshotParallel(b *testing.B) {
	m := &txMetrics{}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = m.snapshotCountMetric()
		}
	})
}
