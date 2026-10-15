package bench

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestCaptureAndOTLPKeepFinalMetrics(t *testing.T) {
	t.Parallel()

	received := make(chan *collectorpb.ExportMetricsServiceRequest, 8)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)

		request := new(collectorpb.ExportMetricsServiceRequest)
		if err != nil || proto.Unmarshal(data, request) != nil {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		received <- request

		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()

	result, err := RunTest(t.Context(), captureFixture(), RunOptions{
		Report: &ReportOptions{}, Logger: NewLogger(io.Discard),
		Metrics: &MetricsConfig{HTTPEndpoint: strings.TrimPrefix(server.URL, "http://"), Insecure: true, Quiet: true},
		Capture: &CaptureOptions{Directory: t.TempDir(), Metrics: true, Interval: time.Hour},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Capture == nil || result.Capture.State != "complete" {
		t.Fatalf("capture + OTLP: capture=%+v", result.Capture)
	}

	if total := result.Metrics["custom_completed"].Total; total == nil || *total != 10 {
		t.Fatalf("final report counter changed: %v", total)
	}

	assertFinalCapturedCounter(t, result.Capture)

	select {
	case request := <-received:
		found := false

		for _, resource := range request.ResourceMetrics {
			for _, scope := range resource.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if metric.Name == "stroppy_custom_completed" {
						found = true

						if metric.GetSum() == nil || len(metric.GetSum().DataPoints) != 1 ||
							metric.GetSum().DataPoints[0].GetAsDouble() != 10 {
							t.Fatalf("OTLP counter changed by local collection: %v", metric)
						}
					}
				}
			}
		}

		if !found {
			t.Fatal("OTLP custom metric lost")
		}
	case <-time.After(time.Second):
		t.Fatal("OTLP not exported")
	}
}

func assertFinalCapturedCounter(t *testing.T, capture *report.Capture) {
	t.Helper()

	file, err := report.OpenCaptureFile(".", capture, "metrics.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	var last report.MetricSample

	for {
		var sample report.MetricSample
		if err := decoder.Decode(&sample); err != nil {
			if err != io.EOF {
				t.Fatal(err)
			}

			break
		}

		last = sample
	}

	if total := last.Metrics["custom_completed"].Total; total == nil || *total != 10 {
		t.Fatalf("final captured counter changed: %v", total)
	}
}
