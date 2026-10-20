package view

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestCatalogOptionsIncludeAllHistory(t *testing.T) {
	t.Parallel()
	history := &report.History{Directory: t.TempDir()}

	for _, run := range []report.Run{
		{Workload: "custom/demo", Driver: "noop", Drivers: []report.Driver{{Name: "primary", Type: "postgres"}}},
		{Workload: "tpcc/tx", Driver: "mysql"},
	} {
		run.Kind, run.Schema, run.Status = report.Kind, report.SchemaVersion, report.StatusCompleted

		run.StartedAt = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
		if _, err := history.Save(&run); err != nil {
			t.Fatal(err)
		}
	}

	response := httptest.NewRecorder()
	Handler(history).ServeHTTP(response,
		httptest.NewRequest(http.MethodGet, "/?workload=custom%2Fdemo&driver=postgres", nil))

	body := response.Body.String()
	for _, option := range []string{"custom/demo", "tpcc/tx", "noop", "postgres", "mysql"} {
		if !strings.Contains(body, `<option value="`+option+`"`) {
			t.Errorf("missing option %q", option)
		}
	}

	if response.Code != http.StatusOK || !strings.Contains(body, `value="postgres" selected`) ||
		strings.Contains(body, `href="/reports/2026-10-20T12-00-00Z-tpcc-tx"`) {
		t.Fatalf("catalog did not retain options while filtering: %d", response.Code)
	}
}

func TestCatalogCaptureFiltersAreIndependent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		change  func(*report.Capture)
		missing string
		metrics bool
		logs    bool
	}{
		{name: "complete", metrics: true, logs: true},
		{name: "recording", change: func(c *report.Capture) { c.State = "recording" }},
		{name: "unfinished", change: func(c *report.Capture) { c.FinishedAt = time.Time{} }},
		{name: "missed-metrics", change: func(c *report.Capture) { c.State, c.MissedSamples = "incomplete", 1 }, logs: true},
		{name: "lost-logs", change: func(c *report.Capture) {
			c.State = "incomplete"
			stream := c.Streams["logs"]
			stream.Dropped = 1
			c.Streams["logs"] = stream
		}, metrics: true},
		{name: "truncated-metrics", change: func(c *report.Capture) {
			stream := c.Streams["metrics"]
			stream.Truncated = 1
			c.Streams["metrics"] = stream
		}, logs: true},
		{name: "disabled-logs", change: func(c *report.Capture) { delete(c.Streams, "logs") }, metrics: true},
		{name: "failed-logs", change: func(c *report.Capture) {
			stream := c.Streams["logs"]
			stream.Error = "write failed"
			c.Streams["logs"] = stream
		}, metrics: true},
		{name: "invalid-log-file", change: func(c *report.Capture) {
			stream := c.Streams["logs"]
			stream.File = "manifest.json"
			c.Streams["logs"] = stream
		}, metrics: true},
		{name: "empty-metrics", change: func(c *report.Capture) {
			stream := c.Streams["metrics"]
			stream.Records = 0
			c.Streams["metrics"] = stream
		}, logs: true},
		{name: "missing-logs", missing: "logs.jsonl", metrics: true},
		{name: "missing-manifest", missing: "manifest.json"},
		{name: "short-metrics", change: func(c *report.Capture) {
			stream := c.Streams["metrics"]
			stream.Bytes++
			c.Streams["metrics"] = stream
		}, logs: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			history := &report.History{Directory: t.TempDir()}

			capture := &report.Capture{
				Schema: report.CaptureSchema, ID: uuid.NewString(), State: "complete", FinishedAt: time.Now(),
				Streams: map[string]report.CaptureStream{
					"metrics": {Enabled: true, File: "metrics.jsonl", Records: 1, Bytes: 3},
					"logs":    {Enabled: true, File: "logs.jsonl"},
				},
			}
			if tc.change != nil {
				tc.change(capture)
			}

			id := writeCatalogCapture(t, history, capture)
			if tc.missing != "" {
				if err := os.Remove(filepath.Join(history.Directory, capture.Directory, tc.missing)); err != nil {
					t.Fatal(err)
				}
			}

			for _, filter := range []struct {
				query string
				want  bool
			}{
				{"", true},
				{"metrics=complete", tc.metrics},
				{"logs=complete", tc.logs},
				{"metrics=complete&logs=complete", tc.metrics && tc.logs},
			} {
				response := httptest.NewRecorder()
				Handler(history).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?"+filter.query, nil))

				if response.Code != http.StatusOK ||
					strings.Contains(response.Body.String(), `href="/reports/`+id+`"`) != filter.want {
					t.Errorf("%s: status %d, match wanted %t", filter.query, response.Code, filter.want)
				}
			}
		})
	}
}

func TestCatalogCaptureFiltersBeforePagination(t *testing.T) {
	t.Parallel()
	history := &report.History{Directory: t.TempDir()}
	capture := &report.Capture{
		Schema: report.CaptureSchema, ID: uuid.NewString(), State: "complete", FinishedAt: time.Now(),
		Streams: map[string]report.CaptureStream{"logs": {Enabled: true, File: "logs.jsonl"}},
	}

	id := writeCatalogCapture(t, history, capture)
	for i := range catalogPageSize + 1 {
		_, err := history.Save(&report.Run{
			Kind: report.Kind, Schema: report.SchemaVersion, Workload: fmt.Sprintf("uncaptured-%d", i),
			Status: report.StatusCompleted, StartedAt: time.Date(2026, 10, 20, 13, 0, i, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	response := httptest.NewRecorder()
	Handler(history).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?logs=complete", nil))

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "1 matching reports") ||
		!strings.Contains(body, `href="/reports/`+id+`"`) || !strings.Contains(body, `value="complete" checked`) ||
		strings.Contains(body, ">Next<") {
		t.Fatalf("capture filter applied after pagination: %d", response.Code)
	}
}

func writeCatalogCapture(t *testing.T, history *report.History, capture *report.Capture) string {
	t.Helper()

	capture.Directory = "captures/" + capture.ID

	directory := filepath.Join(history.Directory, capture.Directory)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}

	manifest, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}

	for name, data := range map[string][]byte{
		"manifest.json": manifest, "metrics.jsonl": []byte("{}\n"), "logs.jsonl": {},
	} {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	id := "2026-10-20T12-00-00Z-captured"
	run := report.Run{
		Kind: report.Kind, Schema: report.SchemaVersion, Workload: "captured", Status: report.StatusCompleted,
		StartedAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), Capture: capture,
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(history.Directory, id+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	return id
}
