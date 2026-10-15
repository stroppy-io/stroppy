package view

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMetricPreviewsAreReadableAndKeepExactValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ raw, want string }{
		{"0", "0"},
		{"8", "8"},
		{"86805", "86,805"},
		{"304.96716137500005", "≈304.967"},
		{"75.254083", "≈75.254"},
		{"3439576.2574634817", "≈3.44M"},
		{"13.283292160883729", "≈13.283"},
		{"0.000000124", "1.24e-07"},
		{"-1234.125", "-1,234.125"},
		{"0.99999999999999999", "≈1"},
		{"1e-400", "≈0"},
	} {
		stat := previewStat("Average", json.Number(tc.raw), "ms")
		if stat.Text != tc.want || stat.Exact != tc.raw || stat.Unit != "ms" {
			t.Errorf("%s: %+v, want %s with original precision", tc.raw, stat, tc.want)
		}
	}

	if missing := previewStat("p99", nil, "ms"); missing.Text != "—" || missing.Unit != "" {
		t.Fatalf("missing value became a measurement: %+v", missing)
	}
}

func TestMetricGroupsUseNaturalOrderAndAlignedStatistics(t *testing.T) {
	t.Parallel()

	document, err := decode([]byte(`{"kind":"run","metrics":{
"custom_q10_duration":{"type":"histogram","unit":"ms","count":1,"average":12.345678,"sum":12.345678},
"custom_q2_duration":{"type":"histogram","unit":"ms","count":3,"average":2.5,"percentiles":{"p99":5}},
"operations_total":{"type":"counter","total":12345},
"speed":{"type":"gauge","unit":"{item}/s","total":123.456789},
"future":{"type":"custom","extra":"kept"}}}`))
	if err != nil {
		t.Fatal(err)
	}

	groups := groupMetrics(metrics(document["metrics"]))
	if len(groups) != 4 || groups[0].Metrics[0].Name != "custom_q2_duration" ||
		groups[0].Metrics[1].Name != "custom_q10_duration" {
		t.Fatalf("metrics not grouped/naturally ordered: %+v", groups)
	}

	stats := groups[0].Metrics[0].Stats
	if len(stats) != 3 || stats[0].Label != "Count" || stats[1].Label != "Average" || stats[2].Label != "p99" {
		t.Fatalf("unaligned histogram preview: %+v", stats)
	}

	if groups[1].Metrics[0].Stats[0].Text != "12,345" || groups[2].Metrics[0].Unit != "item/s" {
		t.Fatal("counter or unit formatting lost")
	}
}

func TestHistogramPreviewOmitsEmptyBucketsButKeepsOverflow(t *testing.T) {
	t.Parallel()

	metric := map[string]any{
		"bounds":        []any{json.Number("1"), json.Number("5"), json.Number("10")},
		"bucket_counts": []any{json.Number("0"), json.Number("2"), json.Number("0"), json.Number("0")},
	}

	plots := appendHistogram(nil, "latency", "scope", metric)
	if len(plots) != 1 || plots[0].HiddenZeroBuckets != 2 || len(plots[0].Bars) != 2 ||
		!strings.Contains(plots[0].Bars[1].Tip, "overflow") || plots[0].Bars[1].Value != "0" {
		t.Fatalf("empty buckets or overflow misrepresented: %+v", plots)
	}
}

func TestMetricSummaryRetainsKeysAndRawPrecision(t *testing.T) {
	t.Parallel()

	data := []byte(`{"kind":"run","metrics":{"insert_duration":{
"type":"histogram","unit":"ms","count":8,"average":304.96716137500005,"sum":2439.7372910000004}}}`)

	var output bytes.Buffer
	if err := Render(&output, data, ""); err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{
		"Distributions", "Insert duration", "insert_duration", "≈304.967",
		"304.96716137500005", "2439.7372910000004", "Complete metric data",
	} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("metric data hidden/lost: %s", text)
		}
	}

	if strings.Contains(output.String(), "<script") {
		t.Fatal("metric readability requires scripts")
	}
}
