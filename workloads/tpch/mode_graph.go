package tpch

import (
	"context"
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/mpp"
)

func (w *workload) declarePerformanceNodes(
	g *bench.Graph, dialect *mpp.Selection, options modeOptions, setup []bench.NodeRef, state *benchmarkReport,
) {
	previous := g.Node("prepare_streams", func(context.Context, *bench.Bench) error {
		return w.prepareModeSeed(options, state)
	}).After(setup[len(setup)-1])

	count := 1
	if options.name == "full" {
		count = 2
	}

	for number := 1; number <= count; number++ {
		run := &benchmarkRun{Number: number}
		state.Runs = append(state.Runs, run)

		g.Group(fmt.Sprintf("run_%d", number), func(group *bench.Graph) {
			if options.name == "full" || options.name == "power" || options.name == "power-query-only" {
				previous = w.declarePower(group, dialect, options, state, run, previous)
			}

			if options.name == "full" || options.name == "throughput" || options.name == "throughput-query-only" {
				previous = w.declareThroughput(group, dialect, options, state, run, previous)
			}
		})
	}
}

func (w *workload) prepareModeSeed(options modeOptions, state *benchmarkReport) error {
	if !options.seedExplicit {
		if w.modeLoadCompleted.IsZero() {
			return inputModeError("load completion timestamp is unavailable")
		}

		state.loadCompleted = w.modeLoadCompleted
		state.LoadCompletedAt = &state.loadCompleted

		seed, err := seedAtLoadCompletion(w.modeLoadCompleted)
		if err != nil {
			return err
		}

		state.Seed = seed
	}

	state.loadComplete = !w.modeLoadCompleted.IsZero()
	if state.Seed+int64(options.streams) >= querySeedModulus {
		return errStreamSeed
	}

	return nil
}

func (w *workload) declarePower(
	g *bench.Graph, dialect *mpp.Selection, options modeOptions, state *benchmarkReport,
	run *benchmarkRun, previous bench.NodeRef,
) bench.NodeRef {
	phase := &benchmarkPhase{Name: "power", Streams: make([]benchmarkStream, 1)}
	run.Power = phase

	queries := g.WorkerNode("power_queries", w.modeQueryInitializer(dialect, state, phase, true),
		bench.OncePerWorker(1), bench.RetainSession())
	if options.name == "power-query-only" {
		return queries.AfterSuccess(previous)
	}

	set := options.firstSet + int64((run.Number-1)*(options.streams+1))
	session := g.Session("power_refresh", bench.DriverRef{})
	rf1 := g.WorkerNode("power_rf1", w.modeRefreshInitializer(dialect, phase, set, 1, "rf1"),
		bench.OncePerWorker(1), bench.RetainSession()).UseSession(session).AfterSuccess(previous)
	queries.AfterSuccess(previous).RunAfterSuccess(rf1)
	rf1.RunAfterReady(queries)

	return g.WorkerNode("power_rf2", w.modeRefreshInitializer(dialect, phase, set, 1, "rf2"),
		bench.OncePerWorker(1), bench.RetainSession()).UseSession(session).AfterSuccess(queries)
}

func (w *workload) declareThroughput(
	g *bench.Graph, dialect *mpp.Selection, options modeOptions, state *benchmarkReport,
	run *benchmarkRun, previous bench.NodeRef,
) bench.NodeRef {
	phase := &benchmarkPhase{Name: "throughput", Streams: make([]benchmarkStream, options.streams)}
	run.Throughput = phase
	queries := g.WorkerNode("throughput_queries", w.modeQueryInitializer(dialect, state, phase, false),
		bench.OncePerWorker(options.streams), bench.RetainSession()).AfterSuccess(previous)
	participants := []bench.NodeRef{queries}

	if options.name != "throughput-query-only" {
		set := options.firstSet
		if options.name == "full" {
			set += int64((run.Number-1)*(options.streams+1) + 1)
		}

		refresh := g.WorkerNode("throughput_refresh",
			w.modeRefreshInitializer(dialect, phase, set, options.streams, ""),
			bench.OncePerWorker(1), bench.RetainSession()).AfterSuccess(previous)
		queries.RunAfterReady(refresh)
		refresh.RunAfterReady(queries)
		participants = append(participants, refresh)
	}

	g.Measure("throughput_window", participants...)

	return g.Join("throughput_join", participants...)
}
