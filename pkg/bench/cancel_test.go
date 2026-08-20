package bench

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
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
	Register(func() Workload { return &cancelDuringSetupWorkload{} })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(
		ctx,
		"test/cancel-setup",
		map[int]*stroppy.DriverConfig{0: {DriverType: stroppy.DriverConfig_DRIVER_TYPE_NOOP}},
		map[string]string{},
		ParamInputs{},
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
	installRuntimeTestRoot(t)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		done <- runScenario(ctx, scenarioSpec{
			executor: "constant-vus",
			vus:      4,
			duration: 10 * time.Second,
		}, func(vu *VU) error {
			<-vu.Context().Done()

			return vu.Context().Err()
		})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runScenario() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runScenario() did not stop after cancellation — workers leaked")
	}
}
