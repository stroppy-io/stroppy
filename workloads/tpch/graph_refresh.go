package tpch

import (
	"context"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/mpp"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch/dbgen"
)

func (w *workload) declareRefreshStreams(
	g *bench.Graph, dialect *mpp.Selection, selection streamSelection, previous []bench.NodeRef,
) []bench.NodeRef {
	return g.Group("workload", func(group *bench.Graph) {
		if selection.count == 1 {
			session := group.Session("refresh", bench.DriverRef{})
			rf1 := group.WorkerNode("rf1", w.graphRefreshInitializer(dialect, selection, "rf1"),
				bench.OncePerWorker(1)).UseSession(session).After(previous...)
			queries := group.WorkerNode("queries", w.streamInitializer(dialect),
				bench.OncePerWorker(1), bench.RetainSession()).AfterSuccess(rf1)
			rf2 := group.WorkerNode("rf2", w.graphRefreshInitializer(dialect, selection, "rf2"),
				bench.OncePerWorker(1)).UseSession(session).AfterSuccess(queries)
			group.Measure("window", rf1, queries, rf2)

			return
		}

		queries := group.WorkerNode("queries", w.streamInitializer(dialect),
			bench.OncePerWorker(selection.count), bench.RetainSession()).After(previous...)
		refresh := group.WorkerNode("refresh", w.graphRefreshInitializer(dialect, selection, ""),
			bench.OncePerWorker(1), bench.RetainSession()).After(previous...)
		queries.AfterReady(refresh)
		group.Measure("window", queries, refresh)
	})
}

func (w *workload) graphRefreshInitializer(
	dialect *mpp.Selection, selection streamSelection, function string,
) bench.WorkerInit {
	return func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
		session, err := modeSession(ctx, b, dialect)
		if err != nil {
			return nil, err
		}

		dbgen.EnsureInit(w.scaleFactor)

		return func(ctx context.Context, worker *bench.Bench) error {
			return w.runGraphRefresh(ctx, worker, session, selection, function)
		}, nil
	}
}

func (w *workload) runGraphRefresh(
	ctx context.Context, b *bench.Bench, session *bench.Session, selection streamSelection, function string,
) error {
	total := float64(selection.count * 2)
	for pair := range selection.count {
		for index, name := range []string{"rf1", "rf2"} {
			if function != "" && name != function {
				continue
			}

			b.Progress(ctx, bench.ProgressUpdate{
				Activity: name, Unit: "refresh_functions", Current: float64(pair*2 + index), Total: &total,
			})

			if w.queryFailures.Load() {
				return bench.Fatal(errRefreshQuery)
			}

			if err := w.runRefresh(ctx, session, selection.refreshSet+int64(pair), name); err != nil {
				return bench.Fatal(err)
			}

			b.Progress(ctx, bench.ProgressUpdate{
				Activity: name, Unit: "refresh_functions", Current: float64(pair*2 + index + 1), Total: &total,
			})
		}
	}

	return nil
}
