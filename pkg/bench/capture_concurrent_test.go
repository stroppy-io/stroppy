package bench

import (
	"io"
	"sync"
	"testing"
	"time"
)

func TestConcurrentCapturesKeepSeparateIdentities(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	ids := make(chan string, 4)

	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			result, err := RunTest(t.Context(), captureFixture(), RunOptions{
				Report: &ReportOptions{RunID: "reused"}, Logger: NewLogger(io.Discard), Metrics: &MetricsConfig{Quiet: true},
				Capture: &CaptureOptions{Directory: directory, Logs: true, Metrics: true, Interval: 25 * time.Millisecond},
			})
			if err != nil {
				t.Errorf("run: %v", err)

				return
			}

			if result.Capture.ReportID != "reused" || result.Capture.RunID != "reused" {
				t.Error("capture provenance lost")
			}

			ids <- result.Capture.ID
		})
	}

	workers.Wait()
	close(ids)

	unique := map[string]bool{}
	for id := range ids {
		if unique[id] {
			t.Fatal("capture UUID reused")
		}

		unique[id] = true
	}

	if len(unique) != 4 {
		t.Fatalf("captures missing: %d", len(unique))
	}
}
