package bench

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestOncePerWorkerDoesNotShareIterations(t *testing.T) {
	var calls [4]atomic.Int64

	test := Test{Name: "per-worker", Define: func(d *Def) error {
		d.Execution.Step("work", func(_ context.Context, b *Bench) error {
			calls[b.Worker()].Add(1)

			if b.Worker() == 3 {
				time.Sleep(20 * time.Millisecond)
			}

			return nil
		}, OncePerWorker(len(calls)))

		return d.Execution.Err()
	}}

	result, err := RunTest(t.Context(), test, RunOptions{Report: &ReportOptions{}})
	if err != nil {
		t.Fatal(err)
	}

	for worker := range calls {
		if got := calls[worker].Load(); got != 1 {
			t.Fatalf("worker %d executed %d times, want 1", worker, got)
		}
	}

	if result.Scenario.IterationsRequested == nil ||
		*result.Scenario.IterationsRequested != int64(len(calls)) || result.Scenario.DurationSeconds != nil {
		t.Fatalf("unexpected finite-worker scenario: %#v", result.Scenario)
	}
}
