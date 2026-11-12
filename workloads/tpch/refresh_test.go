package tpch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch/tpchgen"
)

func TestRefreshSelection(t *testing.T) {
	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		params map[string]string
		valid  bool
	}{
		{map[string]string{"refresh-functions": "true"}, true},
		{map[string]string{"refresh-functions": "true", "streams": "2"}, true},
		{map[string]string{"refresh-functions": "true", "query-stream": "0"}, true},
		{map[string]string{"refresh-functions": "true", "query-stream": "1"}, false},
		{map[string]string{"refresh-functions": "true", "streams": "2", "refresh-set": "4000"}, false},
		{map[string]string{"refresh-functions": "true", "executor": "constant-vus"}, false},
		{map[string]string{"refresh-set": "1"}, false},
		{map[string]string{"refresh-functions": "true", "pg-unlogged": "true"}, false},
	} {
		_, err := catalog.ResolveRun(Test.Name, bench.RunOptions{
			Params:  bench.ParamInputs{CLI: test.params},
			Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverPostgres}},
		})
		if (err == nil) != test.valid {
			t.Fatalf("params=%v error=%v valid=%t", test.params, err, test.valid)
		}
	}

	_, err = catalog.ResolveRun(Test.Name, bench.RunOptions{
		Params: bench.ParamInputs{CLI: map[string]string{"refresh-functions": "true"}},
		Steps:  []string{"workload"}, Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverPostgres}},
	})
	if !errors.Is(err, errRefreshExisting) {
		t.Fatalf("existing data error=%v", err)
	}

	for _, test := range []struct {
		set, streams string
		valid        bool
	}{
		{"998", "2", true}, {"999", "1", true}, {"999", "2", false}, {"1000", "1", false},
	} {
		_, err := catalog.ResolveRun(Test.Name, bench.RunOptions{
			Params: bench.ParamInputs{CLI: map[string]string{
				"refresh-functions": "true", "refresh-set": test.set, "streams": test.streams,
			}},
			Steps:   []string{"workload"},
			Drivers: map[string]bench.DriverConfig{"default": {Kind: bench.DriverPostgres}},
		})
		if (err == nil) != test.valid {
			t.Fatalf("refresh-set=%s streams=%s valid=%t error=%v", test.set, test.streams, test.valid, err)
		}
	}
}

func TestRefreshWindowIncludesLastActivity(t *testing.T) {
	start := time.Now()
	for _, refreshLast := range []bool{false, true} {
		queryEnd, refreshEnd := start.Add(5*time.Second), start.Add(3*time.Second)
		if refreshLast {
			queryEnd, refreshEnd = refreshEnd, queryEnd
		}

		w := &workload{
			streams: []queryStream{{Complete: true, Succeeded: 22, started: start.Add(time.Second), finished: queryEnd}},
			refresh: &refreshState{Pairs: 1, NextSet: 2, Attempts: []refreshAttempt{
				{Function: "rf1", Status: "completed", started: start, finished: start.Add(time.Second)},
				{Function: "rf2", Status: "completed", started: start.Add(2 * time.Second), finished: refreshEnd},
			}},
		}

		contribution, err := w.streamContribution(bench.ReportContext{Status: report.StatusCompleted})
		if err != nil {
			t.Fatal(err)
		}

		value := contribution.Data.(streamReport)
		if value.MeasurementSeconds != 5 || value.QueriesPerHour == nil || value.TPCCompliant {
			t.Fatalf("window=%+v", value)
		}

		w.refresh.Attempts[1].Status = "failed"

		contribution, err = w.streamContribution(bench.ReportContext{Status: report.StatusCompleted})
		if err != nil {
			t.Fatal(err)
		}

		if contribution.Data.(streamReport).QueriesPerHour != nil {
			t.Fatal("failed refresh has rate")
		}

		contribution, err = w.refreshContribution(bench.ReportContext{Status: report.StatusCompleted})
		if err != nil {
			t.Fatal(err)
		}

		if contribution.Data.(refreshState).NextSet != 0 {
			t.Fatal("failed dataset advertised safe continuation")
		}
	}
}

func TestRefreshInsertFailureRollsBack(t *testing.T) {
	generator, err := tpchgen.NewRefreshGenerator(0.01, 1)
	if err != nil {
		t.Fatal(err)
	}

	order, _ := generator.Next()
	rows, lines := refreshRows(order)
	backend := errors.New("line insert failed")
	recorder := &record.Recorder{}
	test := bench.Test{Name: "refresh-rollback", Define: func(d *bench.Def) error {
		d.Execution.WorkerStep("workload", func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
			session, err := b.Session(ctx)
			if err != nil {
				return nil, err
			}

			return func(ctx context.Context, _ *bench.Bench) error {
				err := session.BeginTx(ctx, bench.BeginOpts{Isolation: bench.IsoSerializable}, func(tx *bench.Tx) error {
					if err := insertRefreshRows(ctx, tx, "orders", refreshOrderColumns, [][]any{rows}); err != nil {
						return err
					}

					if err := insertRefreshRows(ctx, tx, "lineitem", refreshLineColumns, lines); err != nil {
						return err
					}

					return backend
				})

				return bench.Fatal(err)
			}, nil
		}, bench.OncePerWorker(1))

		return d.Execution.Err()
	}}

	_, err = testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	if !errors.Is(err, backend) {
		t.Fatalf("error=%v", err)
	}

	commit, rollback := 0, 0

	for _, operation := range recorder.Operations() {
		if operation.Kind == "commit" {
			commit++
		}

		if operation.Kind == "rollback" {
			rollback++
		}
	}

	if commit != 0 || rollback != 1 {
		t.Fatalf("commit=%d rollback=%d", commit, rollback)
	}
}

func TestRefreshExecutionRange(t *testing.T) {
	settings := bench.RunSettings{Executor: "shared-iterations", Workers: 2, Iterations: 2}

	selection := streamSelection{enabled: true, refresh: true, count: 2, seed: 42, refreshSet: 998}
	if err := selection.validate(settings); err != nil {
		t.Fatal(err)
	}

	selection.refreshSet = 999
	if !errors.Is(selection.validate(settings), errRefreshRange) {
		t.Fatal("throughput range crossed unsupported sparse-key boundary")
	}

	if nextRefreshSet(998) != 999 || nextRefreshSet(999) != 0 {
		t.Fatal("exhausted refresh range advertised unsupported continuation")
	}

	w := &workload{}
	if !errors.Is(w.runRefresh(t.Context(), nil, 1000, "rf1"), errRefreshRange) {
		t.Fatal("unsupported set reached database writes")
	}
}

func TestPowerBarrierCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if !errors.Is(waitRefreshPhase(ctx, make(chan struct{})), context.Canceled) {
		t.Fatal("power barrier ignored cancellation")
	}
}
