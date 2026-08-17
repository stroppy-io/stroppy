package bench

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
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
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("runScenario() error = %v, want sentinel", err)
	}
	if !IsFatalError(err) {
		t.Fatalf("runScenario() error = %T, want FatalError", err)
	}
}

func TestRunScenarioKeepsOrdinaryErrorsPerWorker(t *testing.T) {
	installRuntimeTestRoot(t)

	err := runScenario(context.Background(), scenarioSpec{
		executor:   "shared-iterations",
		vus:        1,
		iterations: 1,
	}, func(*VU) error {
		return errors.New("iteration")
	})
	if err != nil {
		t.Fatalf("runScenario() error = %v, want nil", err)
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
	}, func(*VU) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runScenario() error = %v, want context.Canceled", err)
	}
}

func installRuntimeTestRoot(t *testing.T) {
	t.Helper()

	oldRoot := root
	testRoot, err := newRootState(zap.NewNop(), context.Background(), nil, &MetricsConfig{})
	if err != nil {
		t.Fatalf("newRootState() error = %v", err)
	}
	root = testRoot

	t.Cleanup(func() {
		testRoot.shutdownMetrics()
		root = oldRoot
	})
}
