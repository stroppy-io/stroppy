package cli

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestReportExportCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id := seedCLIReport(t, time.Now().UTC(), "custom/report", report.StatusCompleted)

	stdout, _, err := reportCLI(t, "reports", "export", id)
	if err != nil || !strings.Contains(stdout, "<!doctype html>") || !strings.Contains(stdout, "custom/report") {
		t.Fatalf("export: %s %v", stdout, err)
	}

	path := filepath.Join(t.TempDir(), "result.html")

	stdout, _, err = reportCLI(t, "reports", "export", id, "--file", path)
	if err != nil || stdout != "" {
		t.Fatalf("file export: %s %v", stdout, err)
	}

	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "Download JSON") {
		t.Fatalf("file: %s %v", data, err)
	}
}

func TestRunHTMLIsIndependentOfHistoryAndJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	test := bench.Test{Name: "html-output", Define: func(d *bench.Def) error {
		d.Execution.Step("workload", func(context.Context, *bench.Bench) error { return nil })

		return d.Execution.Err()
	}}

	catalog, err := bench.NewCatalog(test)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "report.html")

	var stdout, stderr bytes.Buffer

	err = Execute(t.Context(), &Options{Catalog: catalog}, []string{
		"run", test.Name, "-d", "noop", "--no-save-report", "--report-html", path, "--report-format", "json",
	}, &stdout, &stderr)
	if err != nil || !strings.Contains(stdout.String(), `"kind": "run"`) {
		t.Fatalf("HTML+JSON: %v %s", err, stderr.String())
	}

	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), test.Name) {
		t.Fatalf("HTML: %s %v", data, err)
	}

	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".stroppy", "reports")); !os.IsNotExist(err) {
		t.Fatalf("opt-out created history: %v", err)
	}

	err = Execute(t.Context(), &Options{Catalog: catalog}, []string{
		"run", test.Name, "-d", "noop", "--no-save-report", "--report-html",
		filepath.Join(t.TempDir(), "missing", "report.html"),
	}, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("requested HTML failure not returned")
	}
}

func TestReportServerStopsOnCancellation(t *testing.T) {
	t.Parallel()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())

	result := make(chan error, 1)
	go func() {
		result <- serveReports(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("ok"))
		}))
	}()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	response.Body.Close()
	cancel()

	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}
