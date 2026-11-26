package tpch

import (
	"context"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/mpp"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch/dbgen"
)

func modeSession(ctx context.Context, b *bench.Bench, dialect *mpp.Selection) (*bench.Session, error) {
	check := dialect.Step(func(context.Context, *bench.Bench) error { return nil })
	if err := check(ctx, b); err != nil {
		return nil, err
	}

	return b.Session(ctx)
}

func (w *workload) modeQueryInitializer(
	dialect *mpp.Selection, state *benchmarkReport, phase *benchmarkPhase, power bool,
) bench.WorkerInit {
	return func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
		session, err := modeSession(ctx, b, dialect)
		if err != nil {
			return nil, err
		}

		stream := &phase.Streams[b.Worker()]

		stream.ID = b.Worker() + 1
		if power {
			stream.ID = 0
		}

		stream.Seed = state.Seed + int64(stream.ID)

		params, err := streamParams(w.scaleFactor, stream.Seed)
		if err != nil {
			return nil, err
		}

		parameters := w.prepareParams(params)

		return func(ctx context.Context, worker *bench.Bench) error {
			return w.runModeQueries(ctx, worker, session, stream, parameters)
		}, nil
	}
}

func (w *workload) runModeQueries(
	ctx context.Context, b *bench.Bench, session *bench.Session, stream *benchmarkStream,
	parameters map[string]map[string]any,
) error {
	order := streamOrders[stream.ID%len(streamOrders)]

	total := float64(len(order))
	for index, number := range order {
		if err := ctx.Err(); err != nil {
			return err
		}

		name := queryNames[number-1]
		body, _ := w.sql.Query(name, "body")
		b.Progress(ctx, bench.ProgressUpdate{Activity: name, Unit: "queries", Current: float64(index), Total: &total})

		started := time.Now()

		if len(stream.Queries) > 0 {
			last := &stream.Queries[len(stream.Queries)-1]
			last.Seconds += started.Sub(stream.finished).Seconds()
		}

		if stream.started.IsZero() {
			stream.started = started
		}

		rows, err := session.RawRows(ctx, body, parameters[name])
		if err == nil {
			err = ctx.Err()
		}

		stream.finished = time.Now()
		elapsed := stream.finished.Sub(started)
		stream.Queries = append(stream.Queries, benchmarkQuery{
			Name: name, Seconds: elapsed.Seconds(), Failed: err != nil, Rows: rows,
		})
		w.recordAttempt(ctx, name, float64(elapsed)/float64(time.Millisecond), err != nil)
		b.Progress(ctx, bench.ProgressUpdate{
			Activity: name, Unit: "queries", Current: float64(index + 1), Total: &total,
		})

		if err != nil {
			return bench.Fatal(err)
		}
	}

	return nil
}

func (w *workload) modeRefreshInitializer(
	dialect *mpp.Selection, phase *benchmarkPhase, first int64, pairs int, function string,
) bench.WorkerInit {
	return func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
		session, err := modeSession(ctx, b, dialect)
		if err != nil {
			return nil, err
		}

		dbgen.EnsureInit(w.scaleFactor)
		local := &workload{
			scaleFactor: w.scaleFactor, driverType: w.driverType,
			refresh: &refreshState{}, refreshMetrics: w.refreshMetrics,
		}

		return func(ctx context.Context, worker *bench.Bench) error {
			err := local.runModeRefresh(ctx, worker, session, first, pairs, function)
			phase.Refresh = append(phase.Refresh, local.refresh.Attempts...)

			return err
		}, nil
	}
}

func (w *workload) runModeRefresh(
	ctx context.Context, b *bench.Bench, session *bench.Session, first int64, pairs int, function string,
) error {
	for pair := range pairs {
		for _, name := range []string{"rf1", "rf2"} {
			if function != "" && function != name {
				continue
			}

			b.Progress(ctx, bench.ProgressUpdate{Activity: name, Unit: "refresh_pairs", Current: float64(pair)})

			if err := w.runRefresh(ctx, session, first+int64(pair), name); err != nil {
				return bench.Fatal(err)
			}
		}
	}

	return nil
}
