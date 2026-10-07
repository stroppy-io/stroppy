package bench

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

// cancelDuringSetupWorkload blocks Setup until ctx is canceled and surfaces the
// cancellation, modeling a schema/load phase interrupted by a signal.
type cancelDuringSetupWorkload struct{}

func (*cancelDuringSetupWorkload) Name() string                          { return "test/cancel-setup" }
func (*cancelDuringSetupWorkload) Define(*Def) error                     { return nil }
func (*cancelDuringSetupWorkload) Iterate(context.Context, *Bench) error { return nil }
func (*cancelDuringSetupWorkload) Teardown(context.Context, *Bench) error {
	return nil
}

func (*cancelDuringSetupWorkload) Setup(ctx context.Context, _ *Bench) error {
	<-ctx.Done()

	return ctx.Err()
}

// TestRunCancelsSetup verifies a canceled Run context reaches workload Setup
// (schema/load) and the cancellation is reported back out of Run.
func TestRunCancelsSetup(t *testing.T) {
	fixtureRegister(func() Workload { return &cancelDuringSetupWorkload{} })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(
		ctx,
		"test/cancel-setup",
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		ParamInputs{},
		nil,
		nil,
		zap.NewNop(),
		&MetricsConfig{},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

// TestRunScenarioConstantVUsCancellation verifies a fixed-duration
// (constant-vus) scenario stops its workers promptly when the parent context is
// canceled. The iterate body blocks like an in-flight query and only returns
// once its per-VU context is canceled, so a leak would hang runScenario and trip
// the timeout guard.
func TestRunScenarioConstantVUsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	go func() { time.Sleep(20 * time.Millisecond); cancel() }()

	test := Test{Name: "duration-cancel", Define: func(d *Def) error {
		d.Execution.Step("work", func(ctx context.Context, b *Bench) error {
			<-ctx.Done()

			return ctx.Err()
		}, ConstantWorkers(4, 10*time.Second, DrainNoTimeout))

		return d.Execution.Err()
	}}
	done := make(chan error, 1)

	go func() { _, err := RunTest(ctx, test, noopRunOptions()); done <- err }()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runScenario() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runScenario() did not stop after cancellation — workers leaked")
	}
}

// teardownTrackingWorkload blocks Iterate until ctx is canceled and records
// whether Teardown ran (and under what context), so we can assert graceful
// cancellation still performs workload teardown exactly once under a fresh ctx.
type teardownContextKey struct{}

type teardownTrackingWorkload struct {
	teardownCalls       atomic.Int32
	teardownCtxCanceled atomic.Bool
	teardownCtxValue    any
}

func (*teardownTrackingWorkload) Name() string                        { return "test/teardown-on-cancel" }
func (*teardownTrackingWorkload) Define(*Def) error                   { return nil }
func (*teardownTrackingWorkload) Setup(context.Context, *Bench) error { return nil }

func (w *teardownTrackingWorkload) Iterate(ctx context.Context, _ *Bench) error {
	<-ctx.Done()

	return ctx.Err()
}

func (w *teardownTrackingWorkload) Teardown(ctx context.Context, _ *Bench) error {
	w.teardownCalls.Add(1)
	w.teardownCtxValue = ctx.Value(teardownContextKey{})

	if ctx.Err() != nil {
		w.teardownCtxCanceled.Store(true)
	}

	return nil
}

// TestRunTeardownRunsOnCancellation verifies that when a run is canceled,
// workload Teardown still runs exactly once under a non-canceled context that
// preserves caller values, so schema cleanup is not skipped.
func TestRunTeardownRunsOnCancellation(t *testing.T) {
	var wl *teardownTrackingWorkload

	fixtureRegister(func() Workload {
		wl = &teardownTrackingWorkload{}

		return wl
	})

	ctx := context.WithValue(context.Background(), teardownContextKey{}, "preserved")
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := Run(
		ctx,
		"test/teardown-on-cancel",
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		ParamInputs{},
		nil,
		nil,
		zap.NewNop(),
		&MetricsConfig{},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}

	if got := wl.teardownCalls.Load(); got != 1 {
		t.Fatalf("Teardown called %d times, want 1", got)
	}

	if wl.teardownCtxCanceled.Load() {
		t.Fatal("Teardown received a canceled context")
	}

	if got := wl.teardownCtxValue; got != "preserved" {
		t.Fatalf("Teardown context value = %v, want preserved", got)
	}
}
