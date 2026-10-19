package view

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func checkChartScript(t *testing.T, node *html.Node) {
	t.Helper()

	if node.FirstChild == nil || node.FirstChild.Data != chartScript {
		t.Error("unexpected executable report content")
	}
}

func TestChartRuntimePolicyMatchesEmbeddedScript(t *testing.T) {
	t.Parallel()

	if strings.Contains(strings.ToLower(chartScript), "</script") {
		t.Fatal("chart bundle can terminate script element")
	}

	digest := sha256.Sum256([]byte(chartScript))
	expected := "script-src 'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'"
	response := httptest.NewRecorder()
	Handler(&report.History{Directory: t.TempDir()}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	policy := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, expected) || strings.Contains(policy, "script-src 'unsafe-inline'") {
		t.Fatal("server policy does not restrict executable code to bundled runtime")
	}
}

func TestInteractiveDataPreservesSamplesAndStaticValues(t *testing.T) {
	t.Parallel()

	c := bars("custom", "scope", "ms", []chartValue{{
		label: `</script><img src=x onerror=alert(1)>`, value: 1.25, text: "≈1",
	}})

	var data interactiveChart
	if err := json.Unmarshal([]byte(interactiveBars(c)), &data); err != nil {
		t.Fatal(err)
	}

	if data.Series[0].Values[0] != 1.25 || data.Labels[0] != c.Bars[0].Category {
		t.Fatal("interactive chart uses rounded labels as source values")
	}

	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "chart", c); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(output.String(), "<img src=x") || !strings.Contains(output.String(), `data-chart=`) {
		t.Fatal("chart data is executable or missing")
	}

	line := &timeline{Name: "custom", Interval: 1, Omitted: 4, Points: []timelinePoint{
		{Time: "0", Value: "10", Sequence: 1}, {Time: "3", Value: "20", Sequence: 4},
	}, Markers: []timelineMarker{{Elapsed: 2, Label: "drain_start"}}}
	if err := json.Unmarshal([]byte(interactiveTimeline(line)), &data); err != nil {
		t.Fatal(err)
	}

	series := data.Series[0]
	if series.Points[1].Sequence != 4 || series.Points[1].Time != "3" || series.Omitted != 4 || series.Interval != 1 ||
		series.Markers[0].Elapsed != 2 {
		t.Fatal("interactive timeline lost gaps, omissions, or phase annotations")
	}
}
