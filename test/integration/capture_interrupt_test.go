//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestInterruptedCaptureRetainsCompleteRecords(t *testing.T) {
	_, binary := stroppyBinary(t)
	root := t.TempDir()
	command := exec.CommandContext(t.Context(), binary, "run", "SELECT 1", "-d", "noop",
		"--executor", "constant-vus", "--duration", "1m", "--capture-metrics", "--capture-interval", "50ms",
		"--capture-dir", root, "--no-save-report", "--log-level", "warn")
	command.Dir = t.TempDir()

	command.Env = append(os.Environ(), "HOME="+command.Dir)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}

	stopped := false
	defer func() {
		if !stopped {
			command.Process.Kill()
			command.Wait()
		}
	}()

	deadline := time.Now().Add(5 * time.Second)

	var manifestPath string

	for time.Now().Before(deadline) {
		paths, _ := filepath.Glob(filepath.Join(root, "*", "manifest.json"))
		if len(paths) == 1 {
			metricInfo, err := os.Stat(filepath.Join(filepath.Dir(paths[0]), "metrics.jsonl"))
			if err == nil && metricInfo.Size() > 0 {
				manifestPath = paths[0]

				break
			}
		}

		time.Sleep(20 * time.Millisecond)
	}

	if manifestPath == "" {
		t.Fatal("capture did not start")
	}

	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	_ = command.Wait()
	stopped = true

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	var capture report.Capture
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatal(err)
	}

	if capture.State != "recording" || report.CaptureState(".", &capture) != "incomplete" {
		t.Fatalf("interrupted capture claimed completion: %+v", capture)
	}

	file, err := report.OpenCaptureFile(".", &capture, "metrics.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var sample report.MetricSample
	if err := json.NewDecoder(file).Decode(&sample); err != nil {
		t.Fatalf("completed sample unreadable: %v", err)
	}

	if sample.CaptureID != capture.ID {
		t.Fatal("stream identity lost")
	}
}
