package run

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/internal/capture"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestRunJSONOutputCannotReplaceCaptureArtifact(t *testing.T) {
	writer, err := capture.New(capture.Options{Directory: t.TempDir(), Logs: true, Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	writer.Add("logs", report.CapturedLog{Message: "retain"})
	manifest := writer.Close()
	path := filepath.Join(manifest.Directory, "logs.jsonl")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer

	err = writeRunReport(&stdout, reportOutput{file: path}, &report.Run{Kind: report.Kind, Capture: manifest})
	if !errors.Is(err, errReportOutputCollision) {
		t.Fatalf("artifact output collision accepted: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("capture artifact changed: %v", err)
	}
}

func TestCaptureFlagsIndependentAndValidated(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		logs, metrics bool
	}{
		{[]string{"simple", "--capture-logs"}, true, false},
		{[]string{"simple", "--capture-metrics"}, false, true},
		{[]string{"simple", "--capture-logs", "--capture-metrics", "--capture-interval=250ms"}, true, true},
	} {
		parsed, err := parseRunArgs(tc.args)
		if err != nil {
			t.Fatal(err)
		}

		options, err := parsed.report.captureOptions()
		if err != nil || options.Logs != tc.logs || options.Metrics != tc.metrics {
			t.Fatalf("capture options: %+v %v", options, err)
		}

		if options.Interval <= 0 {
			t.Fatal("sampling interval not defaulted")
		}
	}

	for _, args := range [][]string{
		{"simple", "--capture-logs", "--no-report"},
		{"simple", "--capture-metrics", "--no-save-report"},
		{"simple", "--capture-interval=1s"},
		{"simple", "--capture-metrics", "--capture-interval=0s"},
		{"simple", "--capture-logs", "--capture-max-bytes=2"},
	} {
		if _, err := parseRunArgs(args); !errors.Is(err, errCaptureFlags) {
			t.Fatalf("invalid capture accepted: %v %v", args, err)
		}
	}

	parsed, err := parseRunArgs([]string{"simple", "--capture-metrics", "--no-save-report", "--report-file=run.json"})
	if err != nil {
		t.Fatal(err)
	}

	options, err := parsed.report.captureOptions()
	if err != nil || options.Interval != time.Second || options.Directory != "captures" {
		t.Fatalf("explicit-output capture root: %+v %v", options, err)
	}
}
