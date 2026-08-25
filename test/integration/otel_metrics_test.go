//go:build integration

package integration

import (
	"bufio"
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
	name   string
	labels labels
	value  float64
	exact  bool
}

type prometheusSample struct {
	name   string
	labels labels
	value  float64
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

func dockerRemoveReportsMissing(output []byte) bool {
	return strings.Contains(strings.ToLower(string(output)), "no such container")
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

func parsePrometheusSamples(body string) ([]prometheusSample, error) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 1024*1024)

	var samples []prometheusSample
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		sample, ok, err := parsePrometheusSampleLine(scanner.Text())
		if err != nil {
			return nil, fmt.Errorf("parse Prometheus line %d: %w", lineNumber, err)
		}
		if ok {
			samples = append(samples, sample)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan Prometheus metrics: %w", err)
	}

	return samples, nil
}

func parsePrometheusSampleLine(line string) (prometheusSample, bool, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return prometheusSample{}, false, nil
	}

	nameEnd := strings.IndexAny(line, "{ \t")
	if nameEnd <= 0 {
		return prometheusSample{}, false, fmt.Errorf("missing metric value in %q", line)
	}

	sample := prometheusSample{name: line[:nameEnd], labels: labels{}}
	remainder := strings.TrimSpace(line[nameEnd:])
	if strings.HasPrefix(remainder, "{") {
		labelEnd, err := prometheusLabelSetEnd(remainder)
		if err != nil {
			return prometheusSample{}, false, err
		}

		sample.labels, err = parsePrometheusLabels(remainder[1:labelEnd])
		if err != nil {
			return prometheusSample{}, false, err
		}
		remainder = strings.TrimSpace(remainder[labelEnd+1:])
	}

	fields := strings.Fields(remainder)
	if len(fields) == 0 {
		return prometheusSample{}, false, fmt.Errorf("metric %q has no value", sample.name)
	}

	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return prometheusSample{}, false, fmt.Errorf("metric %q value %q: %w", sample.name, fields[0], err)
	}
	sample.value = value

	return sample, true, nil
}

func prometheusLabelSetEnd(input string) (int, error) {
	quoted := false
	escaped := false

	for index := 1; index < len(input); index++ {
		switch {
		case escaped:
			escaped = false
		case quoted && input[index] == '\\':
			escaped = true
		case input[index] == '"':
			quoted = !quoted
		case input[index] == '}' && !quoted:
			return index, nil
		}
	}

	return 0, fmt.Errorf("unterminated label set")
}

func parsePrometheusLabels(input string) (labels, error) {
	parsed := labels{}
	position := skipPrometheusLabelSpace(input, 0)

	for position < len(input) {
		key, value, next, err := parsePrometheusLabel(input, position)
		if err != nil {
			return nil, err
		}
		if _, exists := parsed[key]; exists {
			return nil, fmt.Errorf("duplicate label %q", key)
		}

		parsed[key] = value
		position = skipPrometheusLabelSpace(input, next)
		if position == len(input) {
			break
		}
		if input[position] != ',' {
			return nil, fmt.Errorf("label %q is not followed by a comma", key)
		}

		position = skipPrometheusLabelSpace(input, position+1)
	}

	return parsed, nil
}

func parsePrometheusLabel(input string, position int) (string, string, int, error) {
	equals := strings.IndexByte(input[position:], '=')
	if equals < 0 {
		return "", "", 0, fmt.Errorf("label %q has no value", input[position:])
	}

	equals += position
	key := strings.TrimSpace(input[position:equals])
	if key == "" {
		return "", "", 0, fmt.Errorf("empty label name")
	}

	valueStart := equals + 1
	if valueStart >= len(input) || input[valueStart] != '"' {
		return "", "", 0, fmt.Errorf("label %q value is not quoted", key)
	}

	valueEnd, err := prometheusQuotedValueEnd(input, valueStart)
	if err != nil {
		return "", "", 0, fmt.Errorf("label %q: %w", key, err)
	}

	value, err := strconv.Unquote(input[valueStart : valueEnd+1])
	if err != nil {
		return "", "", 0, fmt.Errorf("label %q: %w", key, err)
	}

	return key, value, valueEnd + 1, nil
}

func prometheusQuotedValueEnd(input string, start int) (int, error) {
	escaped := false

	for position := start + 1; position < len(input); position++ {
		switch {
		case escaped:
			escaped = false
		case input[position] == '\\':
			escaped = true
		case input[position] == '"':
			return position, nil
		}
	}

	return 0, fmt.Errorf("unterminated quoted value")
}

func skipPrometheusLabelSpace(input string, position int) int {
	for position < len(input) && (input[position] == ' ' || input[position] == '\t') {
		position++
	}

	return position
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

func requirePrometheusSampleEqual(
	samples []prometheusSample,
	metricName string,
	wantLabels labels,
	wantValue float64,
) error {
	value, matches := matchingPrometheusSample(samples, metricName, wantLabels)
	if matches == 1 && value == wantValue {
		return nil
	}

	return fmt.Errorf(
		"%s with exact labels %v has value %g across %d samples, want one sample equal to %g",
		metricName,
		wantLabels,
		value,
		matches,
		wantValue,
	)
}

func requirePrometheusSampleAtLeast(
	samples []prometheusSample,
	metricName string,
	wantLabels labels,
	minValue float64,
) error {
	value, matches := matchingPrometheusSample(samples, metricName, wantLabels)
	if matches == 1 && value >= minValue {
		return nil
	}

	return fmt.Errorf(
		"%s with exact labels %v has value %g across %d samples, want one sample >= %g",
		metricName,
		wantLabels,
		value,
		matches,
		minValue,
	)
}

func matchingPrometheusSample(
	samples []prometheusSample,
	metricName string,
	wantLabels labels,
) (float64, int) {
	var value float64
	matches := 0

	for _, sample := range samples {
		if sample.name != metricName || !prometheusLabelsEqual(sample.labels, wantLabels) {
			continue
		}

		value = sample.value
		matches++
	}

	return value, matches
}

func prometheusLabelsEqual(got, want labels) bool {
	if len(got) != len(want) {
		return false
	}

	for key, value := range want {
		if got[key] != value {
			return false
		}
	}

	return true
}

func TestRequirePrometheusSampleMatchesExactNamesAndLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantLabels labels
		wantErr    bool
	}{
		{
			name:    "metric suffix rejected",
			body:    `stroppy_metric_suffix{table_name="accounts"} 7`,
			wantErr: true,
		},
		{
			name:    "wrong label name rejected",
			body:    `stroppy_metric{other_table_name="accounts"} 7`,
			wantErr: true,
		},
		{
			name:    "wrong label value rejected",
			body:    `stroppy_metric{table_name="tellers"} 7`,
			wantErr: true,
		},
		{
			name:    "doubled value rejected",
			body:    `stroppy_metric{table_name="accounts"} 14`,
			wantErr: true,
		},
		{
			name: "duplicate series rejected even when sum matches",
			body: `stroppy_metric{table_name="accounts"} 3
stroppy_metric{table_name="accounts"} 4`,
			wantErr: true,
		},
		{
			name:    "unexpected extra label rejected",
			body:    `stroppy_metric{table_name="accounts",extra="dimension"} 7`,
			wantErr: true,
		},
		{
			name: "exact sample accepted",
			body: `# HELP stroppy_metric test
stroppy_metric{table_name="accounts",escaped="a\\\"b"} 7`,
			wantLabels: labels{"table_name": "accounts", "escaped": `a\"b`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			samples, err := parsePrometheusSamples(test.body)
			if err != nil {
				t.Fatalf("parsePrometheusSamples: %v", err)
			}

			wantLabels := test.wantLabels
			if wantLabels == nil {
				wantLabels = labels{"table_name": "accounts"}
			}

			err = requirePrometheusSampleEqual(samples, "stroppy_metric", wantLabels, 7)
			if (err != nil) != test.wantErr {
				t.Fatalf("requirePrometheusSample() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestRequirePrometheusSampleAtLeastRejectsDuplicates(t *testing.T) {
	t.Parallel()

	samples, err := parsePrometheusSamples(`stroppy_metric{table_name="accounts"} 3
stroppy_metric{table_name="accounts"} 4`)
	if err != nil {
		t.Fatalf("parsePrometheusSamples: %v", err)
	}

	err = requirePrometheusSampleAtLeast(
		samples,
		"stroppy_metric",
		labels{"table_name": "accounts"},
		1,
	)
	if err == nil {
		t.Fatal("requirePrometheusSampleAtLeast accepted duplicate series")
	}
}

func TestDockerRemoveReportsMissing(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "missing container is benign",
			output: "Error response from daemon: No such container: stroppy-otel-test-1",
			want:   true,
		},
		{
			name:   "daemon failure is reported",
			output: "Cannot connect to the Docker daemon",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := dockerRemoveReportsMissing([]byte(test.output)); got != test.want {
				t.Fatalf("dockerRemoveReportsMissing() = %t, want %t", got, test.want)
			}
		})
	}
}
