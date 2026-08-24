package bench

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/stroppy-io/stroppy/pkg/config"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
)

func TestRunScenarioReturnsFatalErrorAndCancelsWorkers(t *testing.T) {
	installRuntimeTestRoot(t)

	sentinel := errors.New("fatal")

	var calls atomic.Int64

	err := runScenario(context.Background(), scenarioSpec{
		executor:   "shared-iterations",
		vus:        4,
		iterations: 100,
	}, func(vu *VU) error {
		if calls.Add(1) == 1 {
			return &FatalError{err: sentinel}
		}

		<-vu.Context().Done()

		return vu.Context().Err()
	}, nil)

	if !errors.Is(err, sentinel) {
		t.Fatalf("runScenario() error = %v, want sentinel", err)
	}

	if !IsFatalError(err) {
		t.Fatalf("runScenario() error = %T, want FatalError", err)
	}
}

func TestRunScenarioContinuesAfterOrdinaryErrors(t *testing.T) {
	installRuntimeTestRoot(t)

	var (
		calls    atomic.Int64
		failures atomic.Int64
	)

	err := runScenario(context.Background(), scenarioSpec{
		executor:   "shared-iterations",
		vus:        1,
		iterations: 5,
	}, func(*VU) error {
		calls.Add(1)

		return errors.New("iteration")
	}, func(*VU, error) {
		failures.Add(1)
	})
	if err != nil {
		t.Fatalf("runScenario() error = %v, want nil", err)
	}

	if calls.Load() != 5 || failures.Load() != 5 {
		t.Fatalf("calls = %d, failures = %d, want 5 each", calls.Load(), failures.Load())
	}
}

func TestRunContinuesAndSummarizesOrdinaryErrors(t *testing.T) {
	registerOrdinaryErrorWorkload()

	core, logs := observer.New(zapcore.WarnLevel)

	err := Run(
		context.Background(),
		"test/ordinary-errors-continue",
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		ParamInputs{CLI: map[string]string{"iterations": "6", "vus": "2"}},
		nil,
		nil,
		zap.New(core),
		&MetricsConfig{},
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	if ordinaryErrorWorkloadRun.calls.Load() != 6 {
		t.Fatalf("iteration calls = %d, want 6", ordinaryErrorWorkloadRun.calls.Load())
	}

	snapshot := root.errorReporter.snapshot()
	if snapshot.terminalErrors != 6 || snapshot.failedIterations != 6 || snapshot.failedQueries != 0 {
		t.Fatalf("error summary = %#v, want six failed iterations", snapshot)
	}

	if got := logs.FilterMessage("nonfatal error; continuing").Len(); got != 1 {
		t.Fatalf("initial nonfatal warnings = %d, want 1", got)
	}

	if got := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); got != 0 {
		t.Fatalf("error-level logs = %d, want 0", got)
	}
}

var (
	registerOrdinaryErrorWorkloadOnce sync.Once
	ordinaryErrorWorkloadRun          *ordinaryErrorWorkload
)

func registerOrdinaryErrorWorkload() {
	registerOrdinaryErrorWorkloadOnce.Do(func() {
		Register(func() Workload {
			ordinaryErrorWorkloadRun = &ordinaryErrorWorkload{}

			return ordinaryErrorWorkloadRun
		})
	})
}

type ordinaryErrorWorkload struct {
	calls atomic.Int64
}

func (*ordinaryErrorWorkload) Name() string                        { return "test/ordinary-errors-continue" }
func (*ordinaryErrorWorkload) Define(*Def) error                   { return nil }
func (*ordinaryErrorWorkload) Setup(context.Context, *Bench) error { return nil }
func (w *ordinaryErrorWorkload) Iterate(context.Context, *Bench) error {
	w.calls.Add(1)

	return errors.New("ordinary iteration failure")
}
func (*ordinaryErrorWorkload) Teardown(context.Context, *Bench) error { return nil }

func TestRunRejectsNegativeQueryTimeout(t *testing.T) {
	installRuntimeTestRoot(t)

	Register(func() Workload { return &paramTestWorkload{name: "test/query-timeout-negative"} })

	err := Run(
		context.Background(),
		"test/query-timeout-negative",
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		ParamInputs{CLI: map[string]string{"query-timeout": "-5s"}},
		nil,
		nil,
		zap.NewNop(),
		&MetricsConfig{},
	)
	if err == nil || !strings.Contains(err.Error(), "query-timeout must not be negative") {
		t.Fatalf("Run() error = %v, want negative query-timeout", err)
	}
}

func TestRunScenarioReturnsParentCancellation(t *testing.T) {
	installRuntimeTestRoot(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runScenario(ctx, scenarioSpec{
		executor:   "shared-iterations",
		vus:        1,
		iterations: 1,
	}, func(*VU) error { return nil }, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runScenario() error = %v, want context.Canceled", err)
	}
}

func TestRunPassesScenarioCancellationToWorkload(t *testing.T) {
	var workload *fatalContextWorkload

	Register(func() Workload {
		workload = &fatalContextWorkload{secondStarted: make(chan struct{})}

		return workload
	})

	err := Run(
		context.Background(),
		"test/fatal-context",
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		ParamInputs{LegacyEnv: map[string]string{"VUS": "2", "ITER": "2"}},
		nil,
		nil,
		zap.NewNop(),
		&MetricsConfig{},
	)
	if !errors.Is(err, workload.fatalErr) {
		t.Fatalf("Run() error = %v, want fatal sentinel", err)
	}

	if !workload.canceled.Load() {
		t.Fatal("workload did not receive scenario cancellation")
	}
}

type fatalContextWorkload struct {
	calls         atomic.Int64
	canceled      atomic.Bool
	secondStarted chan struct{}
	fatalErr      error
}

func (*fatalContextWorkload) Name() string { return "test/fatal-context" }

func (*fatalContextWorkload) Define(*Def) error { return nil }

func (w *fatalContextWorkload) Setup(context.Context, *Bench) error {
	w.fatalErr = errors.New("fatal iteration")

	return nil
}

func (w *fatalContextWorkload) Iterate(ctx context.Context, _ *Bench) error {
	if w.calls.Add(1) == 1 {
		<-w.secondStarted

		return &FatalError{err: w.fatalErr}
	}

	close(w.secondStarted)

	select {
	case <-ctx.Done():
		w.canceled.Store(true)

		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return errors.New("scenario context was not canceled")
	}
}

func (*fatalContextWorkload) Teardown(context.Context, *Bench) error { return nil }

func installRuntimeTestRoot(t *testing.T) {
	t.Helper()

	oldRoot := root

	testRoot, err := newRootState(zap.NewNop(), context.Background(), nil, nil, &MetricsConfig{})
	if err != nil {
		t.Fatalf("newRootState() error = %v", err)
	}

	root = testRoot

	t.Cleanup(func() {
		testRoot.errorReporter.stopAndWait()
		testRoot.shutdownMetrics()

		root = oldRoot
	})
}
