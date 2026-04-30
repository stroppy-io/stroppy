//go:build integration

package integration

import (
	"bytes"
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

type labels map[string]string

type metricExpectation struct {
	prefix   string
	labels   labels
	minValue float64
}

// TestOtelStroppyMetricsNoop proves all Stroppy-owned metrics leave k6 through
// the configured OTEL exporter. It uses the noop driver so the test validates
// the stroppy/k6 metrics plumbing without depending on a database.
func TestOtelStroppyMetricsNoop(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	docker := requireDocker(t)
	repoRoot := findRepoRoot(t)

	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found at %s (run `make build` first): %v", binary, err)
	}

	collectorConfig := writeOtelCollectorConfig(t)
	otlpEndpoint, prometheusURL := startOtelCollector(t, docker, collectorConfig)
	stroppyConfig := writeStroppyOtelConfig(t, otlpEndpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary,
		"run", "./test/integration/testdata/otel_metrics.ts",
		"-f", stroppyConfig,
		"-D", "driverType=noop",
		"-D", "url=noop://metrics",
		"-e", "ROWS=100",
		"--steps", "load_data",
		"--", "--quiet",
	)
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("stroppy run failed: %v\n--- stdout ---\n%s\n--- stderr ---\n%s",
			err, stdout.String(), stderr.String())
	}

	body := waitForMetrics(t, prometheusURL, func(body string) error {
		for _, table := range []string{"numbers_a", "numbers_b"} {
			if err := requirePrometheusSample(body, "stroppy_insert_rows_total", labels{
				"table_name": table,
				"step":       "load_data",
			}, 100); err != nil {
				return err
			}
			if err := requirePrometheusSample(body, "stroppy_insert_rows_per_second", labels{
				"table_name": table,
				"step":       "load_data",
			}, 0); err != nil {
				return err
			}
			if err := requirePrometheusSample(body, "stroppy_insert_duration", labels{
				"table_name": table,
				"step":       "load_data",
			}, 0); err != nil {
				return err
			}
			if err := requirePrometheusSample(body, "stroppy_insert_error_rate", labels{
				"table_name": table,
				"step":       "load_data",
			}, 0); err != nil {
				return err
			}
		}

		expected := []metricExpectation{
			{prefix: "stroppy_run_query_duration", labels: labels{
				"name": "outside_query",
				"type": "metrics",
				"step": "workload",
			}},
			{prefix: "stroppy_run_query_count", labels: labels{
				"name": "outside_query",
				"type": "metrics",
				"step": "workload",
			}, minValue: 1},
			{prefix: "stroppy_run_query_error_rate", labels: labels{
				"name": "outside_query",
				"type": "metrics",
				"step": "workload",
			}},
			{prefix: "stroppy_run_query_qps"},
			{prefix: "stroppy_tx_count", labels: labels{
				"tx_action":    "commit",
				"tx_name":      "metrics_tx",
				"tx_isolation": "none",
				"step":         "workload",
			}, minValue: 1},
			{prefix: "stroppy_tx_tps"},
			{prefix: "stroppy_tx_total_duration", labels: labels{
				"tx_action":    "commit",
				"tx_name":      "metrics_tx",
				"tx_isolation": "none",
				"step":         "workload",
			}},
			{prefix: "stroppy_tx_clean_duration", labels: labels{
				"tx_action":    "commit",
				"tx_name":      "metrics_tx",
				"tx_isolation": "none",
				"step":         "workload",
			}},
			{prefix: "stroppy_tx_commit_rate", labels: labels{
				"tx_action":    "commit",
				"tx_name":      "metrics_tx",
				"tx_isolation": "none",
				"step":         "workload",
			}},
			{prefix: "stroppy_tx_error_rate", labels: labels{
				"name": "metrics_tx",
				"step": "workload",
			}},
			{prefix: "stroppy_tx_queries_per_tx", labels: labels{
				"tx_action":    "commit",
				"tx_name":      "metrics_tx",
				"tx_isolation": "none",
				"step":         "workload",
			}, minValue: 1},
		}
		for _, exp := range expected {
			if err := requirePrometheusSample(body, exp.prefix, exp.labels, exp.minValue); err != nil {
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
		t.Skipf("docker not found; required for OTEL collector integration test: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, docker, "info")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("docker daemon unavailable; required for OTEL collector integration test: %v\n%s", err, string(out))
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

	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer rmCancel()
		_ = exec.CommandContext(rmCtx, docker, "rm", "-f", containerID).Run()
	})

	otlpPort := dockerHostPort(t, docker, containerID, "4318/tcp")
	prometheusPort := dockerHostPort(t, docker, containerID, "8889/tcp")
	prometheusURL = "http://127.0.0.1:" + prometheusPort + "/metrics"

	waitForMetricsEndpoint(t, docker, containerID, prometheusURL)

	return "127.0.0.1:" + otlpPort, prometheusURL
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

func requirePrometheusSample(body, metricPrefix string, wantLabels labels, minValue float64) error {
	re := regexp.MustCompile(`(?m)^(` + regexp.QuoteMeta(metricPrefix) + `[A-Za-z0-9_:]*)(?:\{([^}]*)\})?\s+([-+]?[0-9]*\.?[0-9]+(?:[eE][-+]?[0-9]+)?)$`)
	matches := re.FindAllStringSubmatch(body, -1)
	for _, match := range matches {
		if !prometheusLabelsContainAll(match[2], wantLabels) {
			continue
		}
		value, err := strconv.ParseFloat(match[3], 64)
		if err != nil {
			continue
		}
		if value >= minValue {
			return nil
		}
	}
	return fmt.Errorf("missing %s sample with labels %v and value >= %g", metricPrefix, wantLabels, minValue)
}

func prometheusLabelsContainAll(got string, want labels) bool {
	if len(want) == 0 {
		return true
	}
	if got == "" {
		return false
	}
	for key, value := range want {
		needle := key + `="` + value + `"`
		if !strings.Contains(got, needle) {
			return false
		}
	}
	return true
}
