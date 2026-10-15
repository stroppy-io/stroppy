package view

import (
	"encoding/json"
	"net/url"
	"strconv"
	"testing"
)

func TestCaptureFilterRejectsAmbiguousRanges(t *testing.T) {
	t.Parallel()

	for _, values := range []url.Values{
		{"from": {"NaN"}},
		{"to": {"+Inf"}},
		{"from": {"-1"}},
		{"from": {"2"}, "to": {"1"}},
		{"log_page": {"-1"}},
		{"log_page": {"20"}},
		{"log_page": {"999999999999"}},
	} {
		if err := validateCaptureQuery(values); err == nil {
			t.Errorf("invalid filter accepted: %v", values)
		}
	}

	if err := validateCaptureQuery(url.Values{"from": {"1"}, "to": {"2"}, "log_page": {"1"}}); err != nil {
		t.Fatal(err)
	}
}

func TestHistogramTimelineKeepsCumulativeAverageAndCountSeparate(t *testing.T) {
	t.Parallel()

	samples := make([]any, 0, 2)
	for i := range 2 {
		samples = append(samples, map[string]any{
			"sequence": json.Number(strconv.Itoa(i + 1)), "elapsed_seconds": json.Number(strconv.Itoa(i)),
			"metrics": map[string]any{"latency": map[string]any{
				"type": "histogram", "unit": "ms", "series": []any{map[string]any{
					"count": json.Number(strconv.Itoa(i + 1)), "average": json.Number(strconv.Itoa(25 + i)),
				}},
			}},
		})
	}

	plots, _ := timelines(samples, nil, nil, 1)
	if len(plots) != 2 || plots[0].Unit != "ms" || plots[0].Name != "latency · cumulative average" ||
		plots[1].Unit != "observations" || plots[1].Name != "latency · cumulative observations" {
		t.Fatalf("histogram statistics conflated: %+v", plots)
	}
}

func TestNearbyCaptureLogsPreservesFilters(t *testing.T) {
	t.Parallel()

	query := url.Values{
		"metric": {"custom_latency"}, "level": {"warn"}, "step": {"measured"}, "text": {"query failed"},
		"from": {"1"}, "to": {"2"}, "log_page": {"3"},
	}

	link, err := url.Parse(nearbyCaptureLogs(query, 10, 0.5))
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"metric", "level", "step", "text"} {
		if link.Query().Get(key) != query.Get(key) {
			t.Fatalf("nearby link lost %s: %s", key, link)
		}
	}

	if link.Fragment != "captured-logs" || link.Query().Get("from") != "9.500" ||
		link.Query().Get("to") != "10.500" || link.Query().Get("log_page") != "0" {
		t.Fatalf("nearby link range or pagination incorrect: %s", link)
	}

	if query.Get("from") != "1" || query.Get("to") != "2" || query.Get("log_page") != "3" {
		t.Fatal("nearby link mutated current filters")
	}
}

func TestDownsampledTimelineDoesNotDrawAcrossOmittedObservations(t *testing.T) {
	t.Parallel()

	group := &timeline{}
	for i := range 1000 {
		group.Points = append(group.Points, timelinePoint{X: float64(i), Y: float64(i % 10), Sequence: uint64(i + 1)})
	}

	projectTimeline(group, nil, 1)

	if group.Omitted == 0 || len(group.Points) > maxTimelinePoints+1 || len(group.Segments) != 0 {
		t.Fatalf("downsample presented as continuous complete history: %+v", group)
	}
}
