package bench

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
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

const teardownLifecycleDriverType config.DriverType = 1000

type driverTeardownContextKey struct{}

var (
	registerTeardownLifecycleOnce sync.Once
	teardownLifecycleDriverRun    *teardownLifecycleDriver
	teardownLifecycleWorkloadRun  *teardownLifecycleWorkload
	errTeardownLifecycleBegin     = errors.New("test driver does not support transactions")
)

func registerTeardownLifecycleTest() {
	registerTeardownLifecycleOnce.Do(func() {
		driver.RegisterDriver(teardownLifecycleDriverType, func(context.Context, driver.Options) (driver.Driver, error) {
			if teardownLifecycleDriverRun == nil {
				return &teardownLifecycleDriver{}, nil
			}

			return teardownLifecycleDriverRun, nil
		})
		Register(func() Workload {
			if teardownLifecycleWorkloadRun == nil {
				return &teardownLifecycleWorkload{}
			}

			return teardownLifecycleWorkloadRun
		})
	})
}

func TestRunFinalizesDriverAfterWorkload(t *testing.T) {
	registerTeardownLifecycleTest()

	setupErr := errors.New("setup sentinel")
	workloadErr := errors.New("workload teardown sentinel")
	driverErr := errors.New("driver teardown sentinel")
	recorder := &teardownLifecycleRecorder{}

	teardownLifecycleWorkloadRun = &teardownLifecycleWorkload{
		recorder:    recorder,
		setupErr:    setupErr,
		teardownErr: workloadErr,
	}
	teardownLifecycleDriverRun = &teardownLifecycleDriver{
		recorder:    recorder,
		teardownErr: driverErr,
	}

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), driverTeardownContextKey{}, "retained"))
	cancel()

	err := Run(
		ctx,
		"test/driver-teardown-lifecycle",
		map[int]*config.DriverConfig{0: {DriverType: teardownLifecycleDriverType}},
		ParamInputs{},
		nil,
		nil,
		zap.NewNop(),
		&MetricsConfig{},
	)
	for _, want := range []error{setupErr, workloadErr, driverErr} {
		if !errors.Is(err, want) {
			t.Errorf("Run() error = %v, want joined %v", err, want)
		}
	}

	if got := strings.Join(recorder.order, ","); got != "workload,driver" {
		t.Fatalf("teardown order = %q, want workload,driver", got)
	}

	if recorder.workloadCalls != 1 || recorder.driverCalls != 1 {
		t.Fatalf("teardown calls = workload:%d driver:%d, want one each", recorder.workloadCalls, recorder.driverCalls)
	}

	if !recorder.workloadDetached || !recorder.driverDetached {
		t.Fatalf("detached contexts = workload:%t driver:%t, want true", recorder.workloadDetached, recorder.driverDetached)
	}

	if !recorder.workloadDeadline || !recorder.driverDeadline {
		t.Fatalf("deadline contexts = workload:%t driver:%t, want true", recorder.workloadDeadline, recorder.driverDeadline)
	}

	if teardownLifecycleWorkloadRun.contextValue != "retained" || teardownLifecycleDriverRun.contextValue != "retained" {
		t.Fatalf(
			"teardown context values = workload:%v driver:%v, want retained",
			teardownLifecycleWorkloadRun.contextValue,
			teardownLifecycleDriverRun.contextValue,
		)
	}
}

type teardownLifecycleRecorder struct {
	order            []string
	workloadCalls    int
	driverCalls      int
	workloadDetached bool
	driverDetached   bool
	workloadDeadline bool
	driverDeadline   bool
}

type teardownLifecycleWorkload struct {
	recorder     *teardownLifecycleRecorder
	setupErr     error
	teardownErr  error
	contextValue any
}

func (*teardownLifecycleWorkload) Name() string                          { return "test/driver-teardown-lifecycle" }
func (*teardownLifecycleWorkload) Define(*Def) error                     { return nil }
func (w *teardownLifecycleWorkload) Setup(context.Context, *Bench) error { return w.setupErr }
func (*teardownLifecycleWorkload) Iterate(context.Context, *Bench) error { return nil }
func (w *teardownLifecycleWorkload) Teardown(ctx context.Context, _ *Bench) error {
	if w.recorder == nil {
		return w.teardownErr
	}

	w.recorder.order = append(w.recorder.order, "workload")
	w.recorder.workloadCalls++
	w.recorder.workloadDetached = ctx.Err() == nil
	_, w.recorder.workloadDeadline = ctx.Deadline()
	w.contextValue = ctx.Value(driverTeardownContextKey{})

	return w.teardownErr
}

type teardownLifecycleDriver struct {
	recorder     *teardownLifecycleRecorder
	teardownErr  error
	contextValue any
}

func (*teardownLifecycleDriver) Insert(context.Context, *driver.InsertRequest) (*stats.Query, error) {
	return &stats.Query{}, nil
}

func (*teardownLifecycleDriver) RunQuery(context.Context, string, map[string]any) (*driver.QueryResult, error) {
	return &driver.QueryResult{}, nil
}

func (*teardownLifecycleDriver) Begin(context.Context, config.TxIsolationLevel) (driver.Tx, error) {
	return nil, errTeardownLifecycleBegin
}

func (*teardownLifecycleDriver) ClassifyError(err error) driver.ErrorFacts {
	return driver.DefaultErrorFacts(err)
}

func (d *teardownLifecycleDriver) Teardown(ctx context.Context) error {
	if d.recorder == nil {
		return d.teardownErr
	}

	d.recorder.order = append(d.recorder.order, "driver")
	d.recorder.driverCalls++
	d.recorder.driverDetached = ctx.Err() == nil
	_, d.recorder.driverDeadline = ctx.Deadline()
	d.contextValue = ctx.Value(driverTeardownContextKey{})

	return d.teardownErr
}

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

func TestRunQuietSummaryDeliversMetricsSilently(t *testing.T) {
	registerQuietSummaryWorkloadOnce.Do(func() {
		Register(func() Workload { return &noopIterateWorkload{} })
	})

	var captured metricdata.ResourceMetrics

	oldStderr := os.Stderr

	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	os.Stderr = pipeWrite

	runErr := Run(
		context.Background(),
		"test/quiet-summary",
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		ParamInputs{CLI: map[string]string{"iterations": "7", "vus": "1"}},
		nil,
		nil,
		zap.NewNop(),
		&MetricsConfig{
			Quiet: true,
			OnSummary: func(data metricdata.ResourceMetrics) {
				captured = data
			},
		},
	)

	pipeWrite.Close()

	os.Stderr = oldStderr

	printed, _ := io.ReadAll(pipeRead)

	pipeRead.Close()

	if runErr != nil {
		t.Fatalf("Run() error = %v", runErr)
	}

	if len(printed) != 0 {
		t.Fatalf("quiet run printed %q to stderr, want silence", printed)
	}

	if captured.ScopeMetrics == nil {
		t.Fatal("OnSummary did not receive the final snapshot")
	}

	var iterations float64

	for _, scope := range captured.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "stroppy_iterations_total" {
				continue
			}

			if sum, ok := metric.Data.(metricdata.Sum[float64]); ok {
				for _, point := range sum.DataPoints {
					iterations += point.Value
				}
			}
		}
	}

	if iterations != 7 {
		t.Fatalf("iterations_total = %v, want 7", iterations)
	}
}

type noopIterateWorkload struct{}

var registerQuietSummaryWorkloadOnce sync.Once

func (*noopIterateWorkload) Name() string                        { return "test/quiet-summary" }
func (*noopIterateWorkload) Define(*Def) error                   { return nil }
func (*noopIterateWorkload) Setup(context.Context, *Bench) error { return nil }
func (*noopIterateWorkload) Iterate(context.Context, *Bench) error {
	return nil
}
func (*noopIterateWorkload) Teardown(context.Context, *Bench) error { return nil }
