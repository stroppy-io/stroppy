package bench

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func captureFixture() Test {
	return Test{Name: "capture-fixture", Define: func(d *Def) error {
		counter := d.Metrics.Counter("custom_completed")
		d.Drivers.Declare("default", DriverConfig{Kind: DriverNoop})
		d.Execution.Step("workload", func(ctx context.Context, b *Bench) error {
			counter.Add(ctx, 1)
			b.Log.Info("iteration", "iteration", b.Iteration())
			time.Sleep(10 * time.Millisecond)

			return nil
		}, SharedIterations(1, 10))

		return d.Execution.Err()
	}}
}

//nolint:nestif // each enabled stream has independent artifact assertions.
func TestCaptureDoesNotChangeFinalCountersAndIsIndependent(t *testing.T) {
	for _, tc := range []struct {
		name          string
		logs, metrics bool
	}{{"logs", true, false}, {"metrics", false, true}, {"both", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := RunTest(t.Context(), captureFixture(), RunOptions{
				Report: &ReportOptions{}, Logger: NewLogger(io.Discard), Metrics: &MetricsConfig{Quiet: true},
				Capture: &CaptureOptions{
					Directory: t.TempDir(), Logs: tc.logs, Metrics: tc.metrics, Interval: 15 * time.Millisecond,
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			if result.Capture == nil || result.Capture.State != "complete" || *result.Metrics["custom_completed"].Total != 10 {
				t.Fatalf("capture changed final result: %+v", result)
			}

			if tc.logs {
				assertCapturedWorkloadLogs(t, result.Capture, 10)
			}

			if tc.metrics {
				file, err := os.Open(filepath.Join(result.Capture.Directory, "metrics.jsonl"))
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()

				scanner := bufio.NewScanner(file)
				previous := 0.0

				for scanner.Scan() {
					var sample report.MetricSample
					if json.Unmarshal(scanner.Bytes(), &sample) != nil {
						t.Fatal("invalid sample")
					}

					if metric, ok := sample.Metrics["custom_completed"]; ok && metric.Total != nil {
						if *metric.Total < previous {
							t.Fatal("counter collection not cumulative")
						}

						previous = *metric.Total
					}
				}

				if err := scanner.Err(); err != nil {
					t.Fatal(err)
				}

				if previous != 10 {
					t.Fatalf("final sample missing: %f", previous)
				}
			}
		})
	}
}

func assertCapturedWorkloadLogs(t *testing.T, capture *report.Capture, expected int) {
	t.Helper()

	data, err := report.ReadCaptureFile(".", capture, "logs.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	count := 0

	for decoder.More() {
		var entry report.CapturedLog
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}

		if entry.Message != "run progress" {
			count++
		}
	}

	if count != expected {
		t.Fatalf("workload logs=%d, want %d", count, expected)
	}
}

func TestCaptureDisabledAndReportingRequired(t *testing.T) {
	result, err := RunTest(t.Context(), captureFixture(), RunOptions{
		Report: &ReportOptions{}, Metrics: &MetricsConfig{Quiet: true},
	})
	if err != nil || result.Capture != nil {
		t.Fatalf("implicit recording: %v %v", result.Capture, err)
	}

	_, err = RunTest(t.Context(), captureFixture(), RunOptions{
		Capture: &CaptureOptions{Directory: t.TempDir(), Metrics: true},
	})
	if !errors.Is(err, errCaptureNoReport) {
		t.Fatalf("capture without report accepted: %v", err)
	}
}

func TestCapturedTerminalErrorRetainsStepAndWorker(t *testing.T) {
	test := Test{Name: "capture-errors", Define: func(d *Def) error {
		for _, step := range []string{"warmup", "workload"} {
			d.Execution.Step(step, func(context.Context, *Bench) error {
				return errors.New("iteration failed")
			}, SharedIterations(1, 2))
		}

		return d.Execution.Err()
	}}

	result, err := RunTest(t.Context(), test, RunOptions{
		Report: &ReportOptions{}, Logger: NewLogger(io.Discard), Metrics: &MetricsConfig{Quiet: true},
		Capture: &CaptureOptions{Directory: t.TempDir(), Logs: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := report.ReadCaptureFile(".", result.Capture, "logs.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))

	for _, step := range []string{"warmup", "workload"} {
		var log report.CapturedLog
		for {
			if err := decoder.Decode(&log); err != nil {
				t.Fatal(err)
			}

			if log.Message != "run progress" {
				break
			}
		}

		var fields struct {
			Step   string `json:"step"`
			Worker *int   `json:"worker"`
		}
		if err := json.Unmarshal(log.Fields, &fields); err != nil {
			t.Fatal(err)
		}

		if fields.Step != step || fields.Worker == nil || *fields.Worker != 0 {
			t.Fatalf("terminal error lost execution identity: %s", log.Fields)
		}
	}
}

func TestCaptureFailedAndCanceledRunsCloseArtifacts(t *testing.T) {
	for _, failure := range []error{errors.New("failed"), context.Canceled} {
		test := Test{Name: "capture-failure", Define: func(d *Def) error {
			d.Execution.Step("setup", func(context.Context, *Bench) error { return failure })

			return d.Execution.Err()
		}}

		result, err := RunTest(t.Context(), test, RunOptions{
			Report: &ReportOptions{}, Logger: LoggerFromBackend(zap.NewNop()),
			Metrics: &MetricsConfig{Quiet: true},
			Capture: &CaptureOptions{Directory: t.TempDir(), Metrics: true, Interval: time.Second},
		})
		if err == nil || result.Capture == nil || result.Capture.FinishedAt.IsZero() {
			t.Fatalf("failure recording not finalized: %v %v", result, err)
		}
	}
}
