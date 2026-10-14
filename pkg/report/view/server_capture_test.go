package view

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/internal/capture"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func checkPortableBundle(t *testing.T, bundle []byte, directory string) {
	t.Helper()

	archive, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatal(err)
	}

	if len(archive.File) != 6 {
		t.Fatalf("incomplete rich bundle: %d files", len(archive.File))
	}

	for _, name := range []string{"index.html", "report.json"} {
		file, err := archive.Open(name)
		if err != nil {
			t.Fatal(err)
		}

		data, err := io.ReadAll(file)
		file.Close()

		if err != nil || !bytes.Contains(data, []byte("captures/")) || bytes.Contains(data, []byte(directory)) {
			t.Fatalf("nonportable %s: %v", name, err)
		}
	}
}

func TestCaptureHTTPFiltersArtifactsAndPortableBundle(t *testing.T) {
	t.Parallel()

	writer, err := capture.New(capture.Options{
		Directory: t.TempDir(), Logs: true, Metrics: true, Interval: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, step := range []string{"warmup", "workload"} {
		writer.Add("logs", report.CapturedLog{
			ElapsedSeconds: 1, Level: "warn", Message: step + " failed",
			Fields: []byte(`{"step":"` + step + `","worker":0}`),
		})
	}

	for i := range 2 {
		value := float64(i)
		writer.Add("metrics", report.MetricSample{
			Sequence: uint64(i + 1), ElapsedSeconds: value, Semantics: "cumulative",
			Metrics: map[string]report.Metric{"custom": {
				Type: "counter", Series: []report.MetricSeries{{Total: &value}},
			}},
		})
	}

	history := &report.History{Directory: t.TempDir()}

	path, err := history.Save(&report.Run{
		Kind: report.Kind, Schema: report.SchemaVersion, ID: "custom", Workload: "custom",
		Status: report.StatusCompleted, StartedAt: time.Now(),
		Capture: writer.Close(),
	})
	if err != nil {
		t.Fatal(err)
	}

	id := strings.TrimSuffix(filepath.Base(path), ".json")

	server := httptest.NewServer(Handler(history))
	defer server.Close()

	get := func(path string, status int) []byte {
		t.Helper()

		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()

		data, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != status {
			t.Fatalf("GET %s: status %d, error %v, body %.300s", path, response.StatusCode, err, data)
		}

		return data
	}
	prefix := "/reports/" + id

	page := get(prefix+"?level=warn&step=workload&text=failed&from=0&to=2&metric=custom", http.StatusOK)
	if !bytes.Contains(page, []byte("workload failed")) || bytes.Contains(page, []byte("warmup failed")) ||
		!bytes.Contains(page, []byte("custom · cumulative total")) || bytes.Contains(page, []byte("<script")) {
		t.Fatal("rich report filters or custom metric projection failed")
	}

	get(prefix+"?from=NaN", http.StatusBadRequest)
	get(prefix+"/artifacts/unknown.jsonl", http.StatusUnprocessableEntity)

	logs := get(prefix+"/artifacts/logs.jsonl", http.StatusOK)
	if !bytes.Contains(logs, []byte("warmup failed")) || !bytes.Contains(logs, []byte("workload failed")) {
		t.Fatal("artifact download was filtered")
	}

	bundle := get(prefix+"/bundle", http.StatusOK)

	checkPortableBundle(t, bundle, history.Directory)

	data, err := history.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := report.CaptureFromJSON(data)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(history.Directory, manifest.Directory, "logs.jsonl")); err != nil {
		t.Fatal(err)
	}

	get(prefix+"/artifacts/logs.jsonl", http.StatusNotFound)

	response, err := server.Client().Get(server.URL + prefix + "/bundle")
	if err == nil {
		_, err = io.ReadAll(response.Body)
		response.Body.Close()
	}

	if err == nil {
		t.Fatal("incomplete bundle download completed without an error")
	}
}
