package view

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestComparisonHTTPSelectionExportAndUnknownPayload(t *testing.T) {
	t.Parallel()
	history := &report.History{Directory: t.TempDir()}
	ids := make([]string, 0, 22)

	for i := range 22 {
		value := float64(i)

		path, err := history.Save(&report.Run{
			Schema: report.SchemaVersion, Kind: report.Kind, ID: "run", Workload: "<custom>",
			Status: report.StatusCompleted, StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			Metrics: map[string]report.Metric{"custom": {Type: "gauge", Unit: "s", Total: &value}},
			WorkloadReports: []report.WorkloadReport{{
				Kind: "unknown", Schema: 7, Data: json.RawMessage(`{"text":"<script>unsafe</script>"}`),
			}},
		})
		if err != nil {
			t.Fatal(err)
		}

		ids = append(ids, strings.TrimSuffix(filepath.Base(path), ".json"))
	}

	server := httptest.NewServer(Handler(history))
	defer server.Close()

	get := func(path string, status int) string {
		t.Helper()

		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()

		data, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != status {
			t.Fatalf("GET %s: %d %v %s", path, response.StatusCode, err, data)
		}

		return string(data)
	}
	query := url.Values{"id": {ids[0], ids[21]}, "reference": {ids[21]}}

	page := get("/compare?"+query.Encode(), 200)
	if !strings.Contains(page, "Selected benchmark runs") || !strings.Contains(page, "Run 2") ||
		strings.Contains(page, "<script>unsafe") {
		t.Fatal("comparison page unsafe or incomplete")
	}

	exportURL := comparisonExportLink(t, page, query)

	export := get(exportURL, 200)
	if !strings.Contains(export, "comparison.json") || strings.Contains(export, `href="/reports/`) {
		t.Fatal("offline comparison depends on server")
	}

	catalog := get("/?"+query.Encode(), 200)
	if !strings.Contains(catalog, `type="hidden" name="id" value="`+ids[0]+`"`) ||
		!strings.Contains(catalog, `form="catalog-selection" name="page" value="1"`) {
		t.Fatal("catalog selection lost across pages")
	}

	get("/compare?id="+ids[0], 400)
	get("/compare?id="+ids[0]+"&id="+ids[21]+"&reference=missing", 400)
}

func comparisonExportLink(t *testing.T, page string, query url.Values) string {
	t.Helper()

	document, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}

	ids := map[string]string{}
	export := ""

	var inspect func(*html.Node)

	inspect = func(node *html.Node) {
		for _, attr := range node.Attr {
			if attr.Key == "id" {
				if ids[attr.Val] != "" {
					t.Errorf("duplicate document ID %q", attr.Val)
				}

				ids[attr.Val] = node.Data
			}

			if node.Data == "a" && attr.Key == "href" && strings.HasPrefix(attr.Val, "/compare/export?") {
				export = attr.Val
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			inspect(child)
		}
	}
	inspect(document)

	link, err := url.Parse(export)
	if err != nil || export == "" || link.Query().Encode() != query.Encode() {
		t.Fatalf("export link lost selection: %q %v", export, err)
	}

	if ids["comparison-metrics"] != "section" || ids["comparison-metric-names"] != "datalist" {
		t.Fatal("metric navigation target is not unique")
	}

	return export
}

func TestComparisonTimelineUsesRecordedBounds(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, first, second, minimum, maximum string
	}{
		{"positive", "100", "120", "100", "120"},
		{"negative", "-120", "-100", "-120", "-100"},
		{"constant", "100", "100", "100", "100"},
		{"empty", "", "", "0", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			group := comparisonTimeline{}
			if tc.first != "" {
				group.Series = []comparisonLine{
					{Samples: []timelinePoint{{Time: "0", Value: tc.first}}},
					{Samples: []timelinePoint{{Time: "1", Value: tc.second}}},
				}
			}

			projectComparisonTimeline(&group)

			if group.Minimum != tc.minimum || group.Maximum != tc.maximum {
				t.Fatalf("bounds include unrecorded values: %s..%s", group.Minimum, group.Maximum)
			}

			if tc.first != "" && tc.first != tc.second {
				if group.Series[0].Samples[0].Y != timelineBottom ||
					group.Series[1].Samples[0].Y != timelineBottom-timelineHeight {
					t.Fatal("recorded variation does not span plot")
				}
			}
		})
	}
}

func TestComparisonTimelineKeepsClockAndSequenceGaps(t *testing.T) {
	t.Parallel()

	group := comparisonTimeline{Series: []comparisonLine{
		{Label: "Run 1", Interval: 1, Samples: []timelinePoint{
			{Time: "0", Value: "0", Sequence: 1},
			{Time: "1", Value: "2", Sequence: 2},
			{Time: "3", Value: "3", Sequence: 4},
		}, Markers: []timelineMarker{{Elapsed: 1, Label: "step_start"}}},
		{Label: "Run 2", Interval: 1, Samples: []timelinePoint{
			{Time: "0", Value: "0", Sequence: 1}, {Time: "6", Value: "6", Sequence: 2},
		}},
	}}
	projectComparisonTimeline(&group)

	if len(group.Series[0].Segments) != 2 || len(group.Series[1].Segments) != 2 || group.Elapsed != "6" {
		t.Fatal("missing interval hidden")
	}

	if fmt.Sprintf("%.0f", group.Series[0].Markers[0].X) != "202" {
		t.Fatalf("marker not aligned to common clock: %+v", group)
	}
}
