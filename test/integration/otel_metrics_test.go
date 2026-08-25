//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	defaultOtelCollectorImage = "otel/opentelemetry-collector-contrib:0.120.0"
	envOtelCollectorImage     = "STROPPY_OTEL_COLLECTOR_IMAGE"
)

var dockerContainerIDRe = regexp.MustCompile(`^[0-9a-f]{12,64}$`)

type metricExpectation struct {
	name   string
	labels labels
	value  float64
	exact  bool
}

// TestOtelStroppyMetrics exports native TPC-B load, query, and transaction
// metrics through OTLP and validates their Prometheus representation.
func TestOtelStroppyMetrics(t *testing.T) {
	docker := requireDocker(t)
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	collectorConfig := writeOtelCollectorConfig(t)
	otlpEndpoint, prometheusURL := startOtelCollector(t, docker, collectorConfig)
	stroppyConfig := writeStroppyOtelConfig(t, otlpEndpoint)
	url := envOr(envTmpfsURL, defaultTmpfsURL)

	runStroppy(t, 2*time.Minute,
		"run", "tpcb/tx",
		"-f", stroppyConfig,
		"-d", "pg",
		"-D", "url="+url,
		"--scale-factor", "1",
		"--load-workers", "2",
		"--executor", "shared-iterations",
		"--iterations", "1",
	)

	body := waitForMetrics(t, prometheusURL, func(body string) error {
		samples, err := parsePrometheusSamples(body)
		if err != nil {
			return err
		}

		rows := map[string]float64{
			"pgbench_branches": 1,
			"pgbench_tellers":  10,
			"pgbench_accounts": 100_000,
		}
		for table, count := range rows {
			tableLabels := labels{"job": "stroppy", "table_name": table, "step": "load_data"}
			progressLabels := labels{
				"event":      "completed",
				"job":        "stroppy",
				"method":     "native",
				"row_kind":   "confirmed",
				"table_name": table,
				"step":       "load_data",
			}

			for _, expectation := range []metricExpectation{
				{name: "stroppy_insert_rows_total", labels: tableLabels, value: count, exact: true},
				{name: "stroppy_insert_progress_rows_total", labels: progressLabels, value: count, exact: true},
				{name: "stroppy_insert_progress_rows_per_second", labels: progressLabels},
				{name: "stroppy_insert_duration_milliseconds_count", labels: tableLabels, value: 1, exact: true},
				{name: "stroppy_insert_operations_total", labels: tableLabels, value: 1, exact: true},
			} {
				if err := requirePrometheusExpectation(samples, expectation); err != nil {
					return err
				}
			}
		}

		txLabels := labels{
			"job":          "stroppy",
			"tx_action":    "commit",
			"tx_name":      "tpcb",
			"tx_isolation": "read_committed",
			"step":         "workload",
		}
		expected := []metricExpectation{
			{name: "stroppy_run_query_operations_total", labels: labels{"job": "stroppy", "step": "workload"}, value: 5, exact: true},
			{name: "stroppy_run_query_duration_milliseconds_count", labels: labels{"job": "stroppy", "step": "workload"}, value: 5, exact: true},
			{name: "stroppy_transactions_total", labels: txLabels, value: 1, exact: true},
			{name: "stroppy_tx_commits_total", labels: txLabels, value: 1, exact: true},
			{name: "stroppy_tx_total_duration_milliseconds_count", labels: txLabels, value: 1, exact: true},
			{name: "stroppy_tx_queries_per_tx_count", labels: txLabels, value: 1, exact: true},
			{name: "stroppy_iterations_total", labels: labels{"job": "stroppy"}, value: 1, exact: true},
			{name: "stroppy_iteration_duration_milliseconds_count", labels: labels{"job": "stroppy"}, value: 1, exact: true},
		}
		for _, expectation := range expected {
			if err := requirePrometheusExpectation(samples, expectation); err != nil {
				return err
			}
		}

		return nil
	})

	t.Logf("collector metrics scrape contains expected Stroppy metrics; bytes=%d", len(body))
}

func requireDocker(t *testing.T) string {
	t.Helper()

	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Fatalf("docker not found; required for OTEL collector integration test: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, docker, "info")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker daemon unavailable; required for OTEL collector integration test: %v\n%s", err, string(out))
	}

	return docker
}

func writeOtelCollectorConfig(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "stroppy-otel-")
	if err != nil {
		t.Fatalf("create collector config dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod collector config dir: %v", err)
	}

	path := filepath.Join(dir, "otel-collector.yaml")
	const config = `receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
exporters:
  prometheus:
    endpoint: 0.0.0.0:8889
service:
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [prometheus]
`
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("write collector config: %v", err)
	}
	return path
}

func writeStroppyOtelConfig(t *testing.T, otlpEndpoint string) string {
	t.Helper()

	cfg := map[string]any{
		"version": "1",
		"global": map[string]any{
			"exporter": map[string]any{
				"name": "integration-otel",
				"otlpExport": map[string]any{
					"otlpHttpEndpoint":     otlpEndpoint,
					"otlpEndpointInsecure": true,
					"otlpMetricsPrefix":    "stroppy_",
				},
			},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal stroppy config: %v", err)
	}

	path := filepath.Join(t.TempDir(), "stroppy-otel.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write stroppy config: %v", err)
	}
	return path
}

func startOtelCollector(t *testing.T, docker, configPath string) (otlpEndpoint, prometheusURL string) {
	t.Helper()

	image := os.Getenv(envOtelCollectorImage)
	if image == "" {
		image = defaultOtelCollectorImage
	}

	name := fmt.Sprintf("stroppy-otel-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { removeOtelCollector(t, docker, name) })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, docker,
		"run", "-d",
		"--name", name,
		"--user", "0:0",
		"-v", configPath+":/tmp/otel-collector.yaml:ro,z",
		"-p", "127.0.0.1::4318",
		"-p", "127.0.0.1::8889",
		image,
		"--config=/tmp/otel-collector.yaml",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("start OTEL collector %s: %v\n%s", image, err, string(out))
	}

	containerID := dockerContainerID(string(out))
	if containerID == "" {
		t.Fatalf("docker run returned no container id\n%s", string(out))
	}

	otlpPort := dockerHostPort(t, docker, containerID, "4318/tcp")
	prometheusPort := dockerHostPort(t, docker, containerID, "8889/tcp")
	prometheusURL = "http://127.0.0.1:" + prometheusPort + "/metrics"

	waitForMetricsEndpoint(t, docker, containerID, prometheusURL)

	return "127.0.0.1:" + otlpPort, prometheusURL
}

func removeOtelCollector(t *testing.T, docker, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, docker, "rm", "-f", name).CombinedOutput()
	if ctxErr := ctx.Err(); ctxErr != nil {
		t.Errorf("remove OTEL collector %s timed out: %v\n%s", name, ctxErr, output)

		return
	}
	if err != nil {
		if dockerRemoveReportsMissing(output) {
			return
		}

		t.Errorf("remove OTEL collector %s: %v\n%s", name, err, output)
	}
}

func dockerContainerID(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if dockerContainerIDRe.MatchString(line) {
			return line
		}
	}
	return ""
}

func dockerHostPort(t *testing.T, docker, containerID, containerPort string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, docker, "port", containerID, containerPort).CombinedOutput()
	if err != nil {
		t.Fatalf("docker port %s %s: %v\n%s", containerID, containerPort, err, string(out))
	}

	line := strings.TrimSpace(strings.Split(strings.TrimSpace(string(out)), "\n")[0])
	_, port, err := net.SplitHostPort(line)
	if err != nil {
		t.Fatalf("parse docker port output %q: %v", line, err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("parse docker host port %q: %v", port, err)
	}
	return port
}

func waitForMetricsEndpoint(t *testing.T, docker, containerID, url string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	client := http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("status %s", resp.Status)
		} else {
			lastErr = err
		}
		if running, status := dockerContainerRunning(docker, containerID); !running {
			t.Fatalf("collector container stopped before %s was ready; status=%s; last_err=%v\n--- docker logs ---\n%s",
				url, status, lastErr, dockerLogs(docker, containerID))
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("collector Prometheus endpoint %s not ready: %v\n--- docker logs ---\n%s",
		url, lastErr, dockerLogs(docker, containerID))
}

func dockerContainerRunning(docker, containerID string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, docker, "inspect", "-f", "{{.State.Status}}", containerID).CombinedOutput()
	status := strings.TrimSpace(string(out))
	if err != nil {
		return false, status
	}
	return status == "running", status
}

func dockerLogs(docker, containerID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, docker, "logs", containerID).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("docker logs failed: %v\n%s", err, string(out))
	}
	return string(out)
}

func waitForMetrics(t *testing.T, url string, check func(string) error) string {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	var body string
	var lastErr error
	client := http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %s", resp.Status)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		body = string(data)
		if err := check(body); err == nil {
			return body
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("expected OTEL metrics did not appear at %s: %v\n--- last scrape ---\n%s", url, lastErr, body)
	return ""
}

func requirePrometheusExpectation(samples []prometheusSample, expectation metricExpectation) error {
	if expectation.exact {
		return requirePrometheusSampleEqual(
			samples, expectation.name, expectation.labels, expectation.value,
		)
	}

	return requirePrometheusSampleAtLeast(
		samples, expectation.name, expectation.labels, expectation.value,
	)
}
