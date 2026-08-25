package integration

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

type labels map[string]string

type prometheusSample struct {
	name   string
	labels labels
	value  float64
}

func dockerRemoveReportsMissing(output []byte) bool {
	return strings.Contains(strings.ToLower(string(output)), "no such container")
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

	return 0, errors.New("unterminated label set")
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
		return "", "", 0, errors.New("empty label name")
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

	return 0, errors.New("unterminated quoted value")
}

func skipPrometheusLabelSpace(input string, position int) int {
	for position < len(input) && (input[position] == ' ' || input[position] == '\t') {
		position++
	}

	return position
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
