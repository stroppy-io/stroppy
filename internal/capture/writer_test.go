package capture

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestIndependentCaptureStreamsAndStructuredLogger(t *testing.T) {
	for _, tc := range []struct {
		name          string
		logs, metrics bool
	}{{"logs", true, false}, {"metrics", false, true}, {"both", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			w, err := New(Options{Directory: t.TempDir(), Logs: tc.logs, Metrics: tc.metrics, Interval: time.Second})
			if err != nil {
				t.Fatal(err)
			}

			encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
			original := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(io.Discard), zapcore.InfoLevel))
			logger := w.Logger(original).Named("workload").With(zap.String("step", "load"), zap.Int("worker", 2))
			logger.Debug("not enabled")
			logger.Info("recorded", zap.String("field", "value"))
			w.Add("metrics", map[string]any{"metrics": map[string]any{"custom": 3}})
			w.Event("step_start", "load", "running", 3, 0, 0)

			result := w.Close()
			if result.State != "complete" {
				t.Fatalf("capture state: %+v", result)
			}

			for _, stream := range []string{"logs", "metrics"} {
				expected := uint64(0)
				if (stream == "logs" && tc.logs) || (stream == "metrics" && tc.metrics) {
					expected = 1
				}

				if result.Streams[stream].Records != expected {
					t.Fatalf("%s count: %+v", stream, result.Streams[stream])
				}
			}

			if tc.logs {
				data, err := os.ReadFile(filepath.Join(result.Directory, "logs.jsonl"))
				if err != nil {
					t.Fatal(err)
				}

				var log report.CapturedLog
				if json.Unmarshal(data, &log) != nil || log.Message != "recorded" ||
					!strings.Contains(string(log.Fields), `"worker":2`) {
					t.Fatalf("log fields lost: %s", data)
				}
			}

			again := w.Close()

			again.Streams["logs"] = report.CaptureStream{}
			if w.Close().Streams["logs"].Enabled != tc.logs {
				t.Fatal("manifest not copied")
			}
		})
	}
}

func TestCaptureRetainsExecutionEventBursts(t *testing.T) {
	w, err := New(Options{Directory: t.TempDir(), Logs: true, Metrics: true, Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	if cap(w.streams["events"].queue) != queueRecords || cap(w.streams["metrics"].queue) != metadataQueueRecords {
		t.Fatal("event bursts share the smaller metric queue capacity")
	}

	const events = 128
	for range events {
		w.Event("step_end", "filtered", "skipped", 0, 0, 0)
	}

	result := w.Close()
	if result.State != "complete" || result.Streams["events"].Records != events {
		t.Fatalf("execution event burst lost: %+v", result)
	}
}

func TestCaptureBoundsAndLossAccounting(t *testing.T) {
	w, err := New(Options{
		Directory: t.TempDir(), Logs: true, Metrics: true, Interval: time.Second, MaxBytes: MaxRecordBytes,
	})
	if err != nil {
		t.Fatal(err)
	}

	oversized := strings.Repeat("x", MaxRecordBytes+1)
	w.Add("logs", map[string]string{"message": oversized})

	var writers sync.WaitGroup
	for range 8 {
		writers.Go(func() {
			for range 1000 {
				w.Add("logs", map[string]string{"message": strings.Repeat("a", 1024)})
			}
		})
	}

	writers.Wait()
	w.Add("metrics", map[string]int{"sample": 1})

	result := w.Close()
	if result.State != "incomplete" || result.Streams["logs"].Truncated == 0 ||
		result.Streams["logs"].Bytes > MaxRecordBytes {
		t.Fatalf("limits not accounted: %+v", result)
	}

	if result.Streams["metrics"].Records != 1 {
		t.Fatal("log pressure blocked metrics stream")
	}
}

func TestCaptureWriteFailureIsVisible(t *testing.T) {
	w, err := New(Options{Directory: t.TempDir(), Logs: true, Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	if err := w.streams["logs"].file.Close(); err != nil {
		t.Fatal(err)
	}

	w.Add("logs", map[string]int{"sample": 1})

	result := w.Close()
	if result.State != "incomplete" || result.Streams["logs"].Error == "" {
		t.Fatalf("write failure hidden: %+v", result)
	}
}

func TestUnfinishedManifestRemainsRecording(t *testing.T) {
	w, err := New(Options{Directory: t.TempDir(), Metrics: true, Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	data, err := os.ReadFile(filepath.Join(w.directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	var result report.Capture
	if json.Unmarshal(data, &result) != nil || result.State != "recording" ||
		strings.Contains(string(data), `"finished_at"`) {
		t.Fatalf("unfinished capture claims completion: %s", data)
	}

	finished := w.Close()

	data, err = json.Marshal(finished)
	if err != nil || finished.FinishedAt.IsZero() || !strings.Contains(string(data), `"finished_at"`) {
		t.Fatalf("completed capture missing finish time: %s %v", data, err)
	}
}
