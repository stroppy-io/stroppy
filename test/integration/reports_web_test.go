//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestReportWebCLI(t *testing.T) {
	_, binary := stroppyBinary(t)
	home := t.TempDir()
	invoke := func(args ...string) []byte {
		t.Helper()
		command := exec.CommandContext(t.Context(), binary, args...)
		command.Dir = home
		command.Env = append(os.Environ(), "HOME="+home)
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("%v: %v %s", args, err, stderr.String())
		}

		return stdout.Bytes()
	}
	path := filepath.Join(home, "run.html")
	invoke("run", "SELECT 1", "-d", "noop", "--report-html", path)
	var entries []report.HistoryEntry
	if err := json.Unmarshal(invoke("reports", "list", "-o", "json"), &entries); err != nil || len(entries) != 1 {
		t.Fatalf("history: %v %v", entries, err)
	}
	id := entries[0].ID
	html := invoke("reports", "export", id)
	if !bytes.Contains(html, []byte("<!doctype html>")) || bytes.Contains(html, []byte("<script")) {
		t.Fatal("script-free HTML export unavailable")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	command := exec.CommandContext(ctx, binary, "reports", "serve")
	command.Dir = home
	command.Env = append(os.Environ(), "HOME="+home)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); command.Wait() })
	line := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			line <- scanner.Text()
		}
	}()
	var address string
	select {
	case text := <-line:
		address = strings.TrimPrefix(text, "Report viewer: ")
	case <-time.After(10 * time.Second):
		t.Fatal("server did not announce URL")
	}
	for _, route := range []string{"", "reports/" + id, "reports/" + id + "/json"} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address+route, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %v %s", route, response.StatusCode, err, data)
		}
		if strings.HasSuffix(route, "/json") && !bytes.Equal(data, invoke("reports", "get", id)) {
			t.Fatal("JSON route changed source")
		}
	}
}
