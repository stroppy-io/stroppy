package cli

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestHTMLExportCannotReplaceSourceReport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id := seedCLIReport(t, time.Now().UTC(), "source", report.StatusCompleted)
	path := filepath.Join(os.Getenv("HOME"), ".stroppy", "reports", id+".json")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = reportCLI(t, "reports", "export", id, "--file", path)
	if !errors.Is(err, errReportExportSource) {
		t.Fatalf("source overwrite accepted: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("source report changed")
	}
}

func TestReportServerWaitsForActiveRequestsOnShutdown(t *testing.T) {
	t.Parallel()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	entered, release := make(chan struct{}), make(chan struct{})

	finished := make(chan error, 1)
	go func() {
		finished <- serveReports(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			w.Write([]byte("complete"))
		}))
	}()

	responseDone := make(chan error, 1)

	go func() {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String(), nil)
		if err != nil {
			responseDone <- err

			return
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			responseDone <- err

			return
		}

		data, err := io.ReadAll(response.Body)
		response.Body.Close()

		if string(data) != "complete" {
			err = errors.Join(err, errors.New("response truncated"))
		}

		responseDone <- err
	}()

	<-entered
	cancel()

	select {
	case err := <-finished:
		close(release)
		t.Fatalf("server returned before handler completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	if err := <-responseDone; err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not drain")
	}
}
