package bench

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/recording"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestGraphDeclarationDoesNotExecuteActions(t *testing.T) {
	var calls atomic.Int64

	test := Test{Name: "graph-declaration", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		first := g.Node("first", func(context.Context, *Bench) error {
			calls.Add(1)

			return nil
		})
		second := g.Node("second", func(context.Context, *Bench) error {
			calls.Add(1)

			return nil
		})
		g.Sequence(first, second)
		g.Mode("all", "Run both actions", first, second)

		return g.Run("all")
	}}
	_, err := DescribeTest(test)
	require.NoError(t, err)
	require.Zero(t, calls.Load())
	_, err = RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
}

func TestGraphRejectsReplayTopologyChangeBeforeActions(t *testing.T) {
	var replays, actions atomic.Int64

	test := Test{Name: "graph-replay", Define: func(d *Def) error {
		g := d.Execution.Graph("test")

		name := "first"
		if replays.Add(1) > 1 {
			name = "changed"
		}

		g.Node(name, func(context.Context, *Bench) error {
			actions.Add(1)

			return nil
		})

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.ErrorContains(t, err, "changed between observation and execution")
	require.Zero(t, actions.Load())
}

func TestGraphRejectsReplayBudgetChangeBeforeActions(t *testing.T) {
	var replays, actions atomic.Int64

	test := Test{Name: "graph-budget-replay", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		iterations := replays.Add(1)
		g.Node("work", func(context.Context, *Bench) error {
			actions.Add(1)

			return nil
		}, SharedIterations(1, iterations))

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.ErrorContains(t, err, "changed between observation and execution")
	require.Zero(t, actions.Load())
}

func TestGraphForkJoinOwnsIndependentMeasurement(t *testing.T) {
	var (
		mu    sync.Mutex
		order []string
	)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	started := make(chan string, 3)
	release := make(chan struct{})
	test := Test{Name: "graph-fork", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		action := func(name string) Action {
			return func(ctx context.Context, _ *Bench) error {
				started <- name

				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}

				mu.Lock()

				order = append(order, name)
				mu.Unlock()

				return nil
			}
		}
		left := g.Node("left", action("left"), SharedIterations(1, 2))
		right := g.Node("right", action("right"), SharedIterations(1, 1))
		joined := g.Join("joined", left, right)
		g.Node("after", func(context.Context, *Bench) error {
			mu.Lock()

			order = append(order, "joined")
			mu.Unlock()

			return nil
		}).AfterSuccess(joined)

		return g.Run("")
	}}

	joined := make(chan struct{})
	go func() {
		defer close(joined)

		seen := make(map[string]bool)
		for len(seen) < 2 {
			select {
			case name := <-started:
				seen[name] = true
			case <-ctx.Done():
				return
			}
		}

		close(release)
	}()

	result, err := RunTest(ctx, test, RunOptions{Metrics: &MetricsConfig{Quiet: true}, Report: &ReportOptions{}})

	cancel()
	<-joined
	require.NoError(t, err)
	require.Equal(t, "joined", order[len(order)-1])
	require.EqualValues(t, 2, result.Executions["left"].Iterations)
	require.EqualValues(t, 1, result.Executions["right"].Iterations)
	require.NotContains(t, result.Executions, "joined")
}

func TestGraphJoinRejectsActionOnlyDependencies(t *testing.T) {
	for _, ready := range []bool{false, true} {
		var calls atomic.Int64

		test := Test{Name: "graph-join-action", Define: func(d *Def) error {
			g := d.Execution.Graph("test")
			first := g.Node("first", func(context.Context, *Bench) error {
				calls.Add(1)

				return nil
			})
			second := g.Node("second", func(context.Context, *Bench) error { return nil })

			joined := g.Join("joined", first)
			if ready {
				joined.RunAfterReady(second)
			} else {
				joined.RunAfterSuccess(second)
			}

			return g.Run("")
		}}
		_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
		require.ErrorContains(t, err, "join cannot have action-only dependencies")
		require.Zero(t, calls.Load())
	}
}

func TestGraphInvalidPlanHasNoSideEffects(t *testing.T) {
	var calls atomic.Int64

	test := Test{Name: "graph-cycle", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		first := g.Node("first", func(context.Context, *Bench) error {
			calls.Add(1)

			return nil
		})
		second := g.Node("second", func(context.Context, *Bench) error {
			calls.Add(1)

			return nil
		})
		first.AfterSuccess(second)
		second.AfterSuccess(first)

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.ErrorContains(t, err, "cycle")
	require.Zero(t, calls.Load())
}

func TestGraphReadinessAndFinally(t *testing.T) {
	var ready, cleanup atomic.Bool

	test := Test{Name: "graph-ready", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		server := g.Node("server", func(ctx context.Context, b *Bench) error {
			ready.Store(true)
			b.Ready()
			<-ctx.Done()

			return ctx.Err()
		}).SignalsReady()
		client := g.Node("client", func(context.Context, *Bench) error {
			require.True(t, ready.Load())

			return nil
		}).AfterReady(server)
		server.While(client)
		g.Node("cleanup", func(ctx context.Context, _ *Bench) error {
			require.NoError(t, ctx.Err())
			cleanup.Store(true)

			return nil
		}).Finally(time.Second, server, client)

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.NoError(t, err)
	require.True(t, cleanup.Load())
}

func TestGraphSessionSurvivesOrderedNodesAndClosesBeforeDependents(t *testing.T) {
	recorder := &record.Recorder{}
	test := Test{Name: "graph-session", Define: func(d *Def) error {
		db := d.Drivers.Declare("default", DriverConfig{Kind: DriverRecording, Recording: recorder})
		g := d.Execution.Graph("test")
		session := g.Session("refresh", db)
		action := func(ctx context.Context, b *Bench) error {
			connection, err := b.Session(ctx)
			if err != nil {
				return err
			}

			return connection.Exec(ctx, "SELECT 1", nil)
		}
		first := g.Node("rf1", action).UseSession(session)
		last := g.Node("rf2", action).UseSession(session).AfterSuccess(first)
		g.Node("after", func(context.Context, *Bench) error {
			closed := 0

			for _, operation := range recorder.Operations() {
				if operation.Kind == "session_close" {
					closed++
				}
			}

			require.Equal(t, 1, closed)

			return nil
		}).AfterSuccess(last)

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.NoError(t, err)

	var ids []uint64

	for _, operation := range recorder.Operations() {
		if operation.Kind == "query" {
			ids = append(ids, operation.Session)
		}
	}

	require.Len(t, ids, 2)
	require.NotZero(t, ids[0])
	require.Equal(t, ids[0], ids[1])
}

func TestGraphSessionClosesPreparedBeforeNextBorrower(t *testing.T) {
	recorder := &record.Recorder{}
	test := Test{Name: "graph-prepared", Define: func(d *Def) error {
		db := d.Drivers.Declare("default", DriverConfig{Kind: DriverRecording, Recording: recorder})
		g := d.Execution.Graph("test")
		session := g.Session("shared", db)
		first := g.Node("first", func(ctx context.Context, b *Bench) error {
			connection, err := b.Session(ctx)
			if err != nil {
				return err
			}

			_, err = connection.Prepare(ctx, QueryHandle{Name: "one", Text: "SELECT 1"})

			return err
		}).UseSession(session)
		g.Node("second", func(ctx context.Context, b *Bench) error {
			closed := 0

			for _, operation := range recorder.Operations() {
				if operation.Kind == "statement_close" {
					closed++
				}
			}

			require.Equal(t, 1, closed)

			connection, err := b.Session(ctx)
			if err != nil {
				return err
			}

			return connection.Exec(ctx, "SELECT 2", nil)
		}).UseSession(session).AfterSuccess(first)

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.NoError(t, err)
}

func TestGraphCapacityDoesNotAddSequentialBranches(t *testing.T) {
	g := (&Execution{}).Graph("capacity")
	action := func(context.Context, *Bench) error { return nil }
	first := g.Node("first", action)
	second := g.Node("second", action).AfterSuccess(first)
	third := g.Node("third", action)
	demands := []sessionDemand{
		{nodes: []*graphNode{first.node}, count: 2},
		{nodes: []*graphNode{second.node}, count: 2},
		{nodes: []*graphNode{third.node}, count: 1},
	}
	require.Equal(t, 3, maximumSessionDemand(demands))
}

func TestGraphCapacityFailsBeforeSetup(t *testing.T) {
	var writes atomic.Int64

	test := Test{Name: "graph-capacity", Define: func(d *Def) error {
		d.Drivers.Declare("default", DriverConfig{Kind: DriverPostgres, Postgres: &PostgresConfig{MaxConns: new(int32(2))}})
		g := d.Execution.Graph("test")
		setup := g.Node("setup", func(context.Context, *Bench) error {
			writes.Add(1)

			return nil
		})
		g.Node("queries", func(context.Context, *Bench) error { return nil },
			OncePerWorker(2), RetainSession()).AfterSuccess(setup)
		g.Node("refresh", func(context.Context, *Bench) error { return nil }, RetainSession()).AfterSuccess(setup)

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.ErrorIs(t, err, ErrSessionCapacity)
	require.Zero(t, writes.Load())
}

func TestGraphReadinessRejectsInitializerSignal(t *testing.T) {
	var actions atomic.Int64

	test := Test{Name: "graph-initializer-ready", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		g.WorkerNode("server", func(_ context.Context, b *Bench) (Action, error) {
			b.Ready()

			return func(context.Context, *Bench) error {
				actions.Add(1)

				return nil
			}, nil
		}).SignalsReady()

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.ErrorContains(t, err, "readiness must be signaled during the action window")
	require.Zero(t, actions.Load())
}

func TestGraphReadinessRequiresAllWorkers(t *testing.T) {
	var ready atomic.Int64

	test := Test{Name: "graph-ready-workers", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		server := g.Node("server", func(ctx context.Context, b *Bench) error {
			ready.Add(1)
			b.Ready()
			<-ctx.Done()

			return ctx.Err()
		}, OncePerWorker(3)).SignalsReady()
		client := g.Node("client", func(context.Context, *Bench) error {
			require.EqualValues(t, 3, ready.Load())

			return nil
		}).AfterReady(server)
		server.While(client)

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.NoError(t, err)
}

func TestGraphMissingReadinessBlocksDependentAndRunsFilteredFinally(t *testing.T) {
	var dependent, cleanup atomic.Bool

	test := Test{Name: "graph-missing-ready", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		first := g.Node("first", func(context.Context, *Bench) error { return nil }).SignalsReady()
		g.Node("dependent", func(context.Context, *Bench) error {
			dependent.Store(true)

			return nil
		}).AfterReady(first)
		g.Node("cleanup", func(context.Context, *Bench) error {
			cleanup.Store(true)

			return nil
		}).Finally(time.Second, first)

		return g.Run("")
	}}
	result, err := RunTest(t.Context(), test, RunOptions{
		NoSteps: []string{"cleanup"}, Metrics: &MetricsConfig{Quiet: true}, Report: &ReportOptions{},
	})
	require.ErrorContains(t, err, "completed without readiness")
	require.False(t, dependent.Load())
	require.True(t, cleanup.Load())

	graphs, decodeErr := result.ExecutionGraphs()
	require.NoError(t, decodeErr)
	require.Equal(t, "blocked", graphs[0].Nodes[1].Status)
}

func TestGraphPanicJoinsAuxiliaryAndFinally(t *testing.T) {
	var stopped, cleaned atomic.Bool

	test := Test{Name: "graph-panic", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		server := g.Node("server", func(ctx context.Context, b *Bench) error {
			b.Ready()
			<-ctx.Done()
			stopped.Store(true)

			return ctx.Err()
		}).SignalsReady()
		client := g.Node("client", func(context.Context, *Bench) error {
			panic("original panic")
		}).AfterReady(server)
		server.While(client)
		g.Node("cleanup", func(context.Context, *Bench) error {
			cleaned.Store(stopped.Load())

			return nil
		}).Finally(time.Second, server, client)

		return g.Run("")
	}}

	require.PanicsWithValue(t, "original panic", func() {
		_, _ = RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	})
	require.True(t, stopped.Load())
	require.True(t, cleaned.Load())
}

func TestGraphCapacityIncludesSharedSessionBetweenBorrowers(t *testing.T) {
	g := (&Execution{}).Graph("capacity")
	action := func(context.Context, *Bench) error { return nil }
	first := g.Node("first", action)
	middle := g.Node("middle", action).AfterSuccess(first)
	last := g.Node("last", action).AfterSuccess(middle)
	require.Equal(t, 3, maximumSessionDemand([]sessionDemand{
		{nodes: []*graphNode{first.node, last.node}, count: 1},
		{nodes: []*graphNode{middle.node}, count: 2},
	}))
}

func TestGraphFatalKeepsRunFailedWhileCancelingSiblings(t *testing.T) {
	failure := errors.New("fatal failure")
	test := Test{Name: "graph-fatal-status", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		started := make(chan struct{})

		g.Node("fatal", func(context.Context, *Bench) error {
			<-started

			return Fatal(failure)
		})
		g.Node("sibling", func(ctx context.Context, _ *Bench) error {
			close(started)
			<-ctx.Done()

			return ctx.Err()
		})

		return g.Run("")
	}}
	result, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}, Report: &ReportOptions{}})
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, context.Canceled)
	require.EqualValues(t, "failed", result.Status)
}

func TestGraphFilteredNodePreservesFailedOrdering(t *testing.T) {
	failure := errors.New("setup failed")

	var writes atomic.Int64

	test := Test{Name: "graph-filter-failure", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		first := g.Node("drop", func(context.Context, *Bench) error { return failure })
		filtered := g.Node("create", func(context.Context, *Bench) error { return nil }).After(first)
		g.Node("load", func(context.Context, *Bench) error {
			writes.Add(1)

			return nil
		}).After(filtered)

		return g.Run("")
	}}
	result, err := RunTest(t.Context(), test, RunOptions{
		NoSteps: []string{"create"}, Metrics: &MetricsConfig{Quiet: true}, Report: &ReportOptions{},
	})
	require.ErrorIs(t, err, failure)
	require.Zero(t, writes.Load())
	require.Equal(t, "blocked", result.Steps[1].Status)
	require.Equal(t, "blocked", result.Steps[2].Status)
}

func TestGraphRejectsAuxiliaryReadinessJoinCycle(t *testing.T) {
	var actions atomic.Int64

	test := Test{Name: "graph-lifetime-cycle", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		action := func(context.Context, *Bench) error {
			actions.Add(1)

			return nil
		}
		owner := g.Node("owner", action)
		barrier := g.Node("barrier", action).AfterCompletion(owner)
		g.Node("auxiliary", action).While(owner).AfterReady(barrier)

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.ErrorContains(t, err, "lifecycle cycle")
	require.Zero(t, actions.Load())
}

func TestGraphUnselectedNodeKeepsOrderingBarrier(t *testing.T) {
	var completed atomic.Bool

	test := Test{Name: "graph-omitted-order", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		first := g.Node("first", func(context.Context, *Bench) error {
			completed.Store(true)

			return nil
		})
		middle := g.Node("omitted", func(context.Context, *Bench) error { return nil }).After(first)
		last := g.Node("last", func(context.Context, *Bench) error {
			if !completed.Load() {
				return errors.New("ordered predecessor still running")
			}

			return nil
		}).After(middle)
		g.Mode("selected", "Omit the intermediate action", first, last)

		return g.Run("selected")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.NoError(t, err)
}

func TestGraphActionBarrierInitializesAllWorkersBeforeWrites(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var initialized, writes atomic.Int64

		failure := errors.New("query initialization failed")
		test := Test{Name: "graph-startup-barrier", Define: func(d *Def) error {
			g := d.Execution.Graph("test")
			queries := g.WorkerNode("queries", func(context.Context, *Bench) (Action, error) {
				initialized.Add(1)

				if fail {
					return nil, failure
				}

				return func(context.Context, *Bench) error { return nil }, nil
			}, OncePerWorker(3))
			refresh := g.WorkerNode("refresh", func(context.Context, *Bench) (Action, error) {
				initialized.Add(1)

				return func(context.Context, *Bench) error {
					if initialized.Load() != 4 {
						return errors.New("refresh started before query initialization")
					}

					writes.Add(1)

					return nil
				}, nil
			})
			queries.RunAfterReady(refresh)
			refresh.RunAfterReady(queries)

			return g.Run("")
		}}

		_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
		if fail {
			require.Error(t, err)
			require.Zero(t, writes.Load())
		} else {
			require.NoError(t, err)
			require.EqualValues(t, 1, writes.Load())
		}
	}
}

func TestGraphJoinedCancellationPreservesIndependentFailure(t *testing.T) {
	failure := errors.New("flush failed")
	test := Test{Name: "graph-joined-cancel", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		server := g.Node("server", func(ctx context.Context, b *Bench) error {
			b.Ready()
			<-ctx.Done()

			return errors.Join(ctx.Err(), failure)
		}).SignalsReady()
		client := g.Node("client", func(context.Context, *Bench) error { return nil }).AfterReady(server)
		server.While(client)

		return g.Run("")
	}}
	result, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}, Report: &ReportOptions{}})
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, context.Canceled)
	require.EqualValues(t, "failed", result.Status)
}

func TestGraphCompletionDependencySurvivesOrdinaryFailure(t *testing.T) {
	failure := errors.New("failed prerequisite")

	var diagnosed atomic.Bool

	test := Test{Name: "graph-diagnostic", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		first := g.Node("first", func(context.Context, *Bench) error { return failure })
		g.Node("diagnostic", func(ctx context.Context, _ *Bench) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			diagnosed.Store(true)

			return nil
		}).AfterCompletion(first)

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.ErrorIs(t, err, failure)
	require.True(t, diagnosed.Load())
}

func TestGraphFailureBlocksDependentAndJoinsCleanup(t *testing.T) {
	failure := errors.New("failed prerequisite")

	var dependent, cleaned atomic.Bool

	test := Test{Name: "graph-failure", Define: func(d *Def) error {
		g := d.Execution.Graph("test")
		first := g.Node("first", func(context.Context, *Bench) error { return failure })
		g.Node("dependent", func(context.Context, *Bench) error {
			dependent.Store(true)

			return nil
		}).AfterSuccess(first)
		g.Node("cleanup", func(context.Context, *Bench) error {
			cleaned.Store(true)

			return nil
		}).Finally(time.Second, first)

		return g.Run("")
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.ErrorIs(t, err, failure)
	require.False(t, dependent.Load())
	require.True(t, cleaned.Load())
}
