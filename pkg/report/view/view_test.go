package view

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

const fixture = `{
"kind":"run","schema":3,"id":"user-id","workload":"custom/demo","status":"completed",
"started_at":"2026-10-13T12:00:00Z","driver":"noop","future":{"large":9007199254740993},
"metrics":{"custom_latency":{"type":"histogram","unit":"ms","series":[
{"attributes":{"step":"warmup"},"bounds":[1,5],"bucket_counts":[10,20,3]},
{"attributes":{"step":"workload"},"bounds":[1,5],"bucket_counts":[5,30,8]}]},
"tps":{"type":"gauge","series":[{"attributes":{"step":"warmup"},"total":100},
{"attributes":{"step":"workload"},"total":42}]}},
"workload_reports":[{"kind":"custom.results","schema":22,"status":"ok","data":{
"workers":[{"name":"alpha","count":5},{"name":"beta","count":9}],
"note":"</script><script>alert(1)</script>"}}]}`

func TestRenderIsCompleteOfflineWithStaticFallback(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Render(&output, []byte(fixture), "history-id"); err != nil {
		t.Fatal(err)
	}

	document, err := html.Parse(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	var (
		scripts, charts, details int
		download                 string
		visit                    func(*html.Node)
	)

	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script":
				scripts++

				checkChartScript(t, n)
			case "svg":
				charts++
			case "details":
				details++
			}

			for _, a := range n.Attr {
				if a.Key == "src" || strings.HasPrefix(a.Key, "on") {
					t.Errorf("unexpected active/external attribute: %+v", a)
				}

				if a.Key == "href" && strings.HasPrefix(a.Val, "data:application/json;base64,") {
					download = strings.TrimPrefix(a.Val, "data:application/json;base64,")
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)

	if scripts != 1 || charts < 2 || details < 5 {
		t.Fatalf("scripts=%d charts=%d details=%d", scripts, charts, details)
	}

	original, err := base64.StdEncoding.DecodeString(download)
	if err != nil || string(original) != fixture {
		t.Fatalf("download changed original JSON: %v", err)
	}

	for _, value := range []string{
		"custom_latency", "custom.results", "9007199254740993", "Identity &amp; configuration", "overflow",
	} {
		if !strings.Contains(output.String(), value) {
			t.Errorf("missing %s", value)
		}
	}
}

func TestChartsPreserveScopeAndOverflow(t *testing.T) {
	t.Parallel()

	p, err := reportPage([]byte(fixture), "")
	if err != nil {
		t.Fatal(err)
	}

	if len(p.Headlines) != 1 || p.Headlines[0].Value.Text != "42" {
		t.Fatalf("warmup mixed into headline: %+v", p.Headlines)
	}

	for _, metric := range p.Metrics {
		if metric.Name != "custom_latency" {
			continue
		}

		if len(metric.Charts) != 2 {
			t.Fatalf("series merged: %+v", metric.Charts)
		}

		if metric.Charts[0].Bars[2].Value != "3" || !strings.Contains(metric.Charts[0].Bars[2].Tip, "overflow") {
			t.Fatal("overflow lost")
		}

		if !strings.Contains(metric.Charts[1].Scope, "step=workload") || !strings.Contains(metric.Charts[1].Scope, "ms") {
			t.Fatal("scope or unit missing")
		}
	}
}

func TestUnfamiliarSchemaAndMalformedData(t *testing.T) {
	t.Parallel()

	p, err := reportPage([]byte(strings.Replace(fixture, `"schema":3`, `"schema":999`, 1)), "")
	if err != nil || p.Warning == "" {
		t.Fatalf("schema warning: %v, %s", err, p.Warning)
	}

	for _, data := range []string{"null", "[]", `{"kind":"other"}`, fixture + " {}"} {
		if err := Render(&bytes.Buffer{}, []byte(data), ""); err == nil {
			t.Errorf("accepted malformed %s", data)
		}
	}
}

func TestCatalogAndReportHTTP(t *testing.T) {
	t.Parallel()
	history := &report.History{Directory: t.TempDir()}

	id := "2026-10-13T12-00-00Z-custom-demo"
	if err := os.WriteFile(filepath.Join(history.Directory, id+".json"), []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}

	handler := Handler(history)

	for _, tc := range []struct {
		path     string
		status   int
		contains string
	}{
		{"/", 200, "custom/demo"},
		{"/?workload=missing", 200, "No matching reports"},
		{"/?page=-1", 400, "invalid catalog page"},
		{"/reports/" + id, 200, "custom_latency"},
		{"/reports/" + id + "/json", 200, fixture},
		{"/reports/2026-10-13T12-00-00Z-missing", 404, "no such file"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))

		if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.contains) {
			t.Errorf("%s: %d %s", tc.path, response.Code, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/reports/"+id, nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("write route available: %d", response.Code)
	}
}

func TestAtomicExport(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.html")
	if err := WriteFile(path, []byte(fixture), ""); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(path, []byte("broken"), ""); err == nil {
		t.Fatal("invalid report export succeeded")
	}

	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed export replaced output")
	}
}

func TestHTMLExportPermissionsAndSymlinkPublication(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	target := filepath.Join(directory, "target.html")
	if err := os.WriteFile(target, []byte("keep target"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(directory, "report.html")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(path, []byte(fixture), ""); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("HTML output mode: %v, %v", info, err)
	}

	if data, err := os.ReadFile(target); err != nil || string(data) != "keep target" {
		t.Fatalf("symlink target changed: %s, %v", data, err)
	}
}

func TestCatalogPagination(t *testing.T) {
	t.Parallel()

	history := &report.History{Directory: t.TempDir()}
	for i := range 21 {
		_, err := history.Save(&report.Run{
			Kind: report.Kind, Schema: 3, Workload: "many", Status: report.StatusCompleted,
			StartedAt: time.Date(2026, 10, 13, 12, 0, i, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	response := httptest.NewRecorder()
	Handler(history).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?page=1", nil))

	if response.Code != 200 || !strings.Contains(response.Body.String(), "Previous") ||
		strings.Contains(response.Body.String(), ">Next<") {
		t.Fatalf("pagination: %s", response.Body.String())
	}
}

func TestHistogramAndSeriesDoNotInventMeasurements(t *testing.T) {
	t.Parallel()

	bad := map[string]any{
		"bounds":        []any{json.Number("5"), json.Number("1")},
		"bucket_counts": []any{json.Number("2"), json.Number("3"), json.Number("4")},
	}
	if plots := appendHistogram(nil, "bad", "scope", bad); len(plots) != 0 {
		t.Fatal("nonmonotonic histogram plotted")
	}

	series := []any{
		map[string]any{"attributes": map[string]any{"step": "warmup"}, "total": json.Number("10")},
		map[string]any{"attributes": map[string]any{"step": "workload"}, "total": json.Number("20")},
	}
	if _, ok := metricSeriesChart("rate", "items/s", series); ok {
		t.Fatal("different phases grouped")
	}
}

func TestCatalogFilterValidation(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/?status=bogus", "/?order=bogus", "/?since=tomorrow"} {
		response := httptest.NewRecorder()
		Handler(&report.History{Directory: t.TempDir()}).ServeHTTP(response,
			httptest.NewRequest(http.MethodGet, path, nil))

		if response.Code != http.StatusBadRequest {
			t.Errorf("%s accepted: %d", path, response.Code)
		}
	}
}

func TestNumericPayloadPrecision(t *testing.T) {
	t.Parallel()

	document, err := decode([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}

	value := document["future"].(map[string]any)["large"].(json.Number)
	if value.String() != "9007199254740993" {
		t.Fatal("identifier rounded")
	}
}
