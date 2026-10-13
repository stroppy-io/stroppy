package view

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestSparseArraysUseLinearGenericView(t *testing.T) {
	t.Parallel()

	items := make([]any, 10_000)
	for i := range items {
		items[i] = map[string]any{fmt.Sprintf("field_%d", i): json.Number("1")}
	}

	result := arrayTree(items)
	if len(result.Columns) != 0 || len(result.Rows) != 0 || len(result.Items) != len(items) {
		t.Fatalf("sparse payload expanded into table: columns=%d rows=%d items=%d",
			len(result.Columns), len(result.Rows), len(result.Items))
	}

	if result.Items[9999].Fields[0].Name != "field_9999" {
		t.Fatal("fallback lost authored data")
	}
}

func TestNonObjectErrorsAndFailureRemainVisible(t *testing.T) {
	t.Parallel()

	for _, errors := range []string{`["original-error"]`, `"original-error"`} {
		data := []byte(`{"kind":"run","schema":999,"errors":` + errors + `,"failure":{"reason":"terminal-failure"}}`)

		var output bytes.Buffer
		if err := Render(&output, data, ""); err != nil {
			t.Fatal(err)
		}

		errorsSection := strings.Split(strings.Split(output.String(), `id="errors"`)[1], `id="data"`)[0]
		if !strings.Contains(errorsSection, "original-error") || !strings.Contains(errorsSection, "terminal-failure") {
			t.Fatalf("structured errors hidden: %s", errorsSection)
		}
	}
}

func TestScopesCannotCollideThroughDisplayLabels(t *testing.T) {
	t.Parallel()

	for _, attributes := range [][2]map[string]any{
		{{"step": "a / b", "phase": "c", "outcome": "d"}, {"step": "a", "phase": "b / c", "outcome": "d"}},
		{{"step": "—"}, {}},
	} {
		series := []any{
			map[string]any{"attributes": attributes[0], "total": json.Number("1")},
			map[string]any{"attributes": attributes[1], "total": json.Number("2")},
		}
		if _, available := metricSeriesChart("custom", "items", series); available {
			t.Fatal("different scope tuples combined")
		}
	}
}

func TestPayloadChartRequiresLabelOnEveryRow(t *testing.T) {
	t.Parallel()

	items := []any{
		map[string]any{"name": "alpha", "count": json.Number("5")},
		map[string]any{"query": "beta", "count": json.Number("9")},
	}
	if _, available := numericObjectArray("queries", items); available {
		t.Fatal("chart invented missing category label")
	}
}

func TestAggregateMetricScopeAndHistogramUnits(t *testing.T) {
	t.Parallel()

	data := []byte(`{"kind":"run","metrics":{"tps":{"type":"gauge","total":142},
"latency":{"type":"histogram","unit":"ms","bounds":[1,5],"bucket_counts":[2,3,1]}}}`)

	p, err := reportPage(data, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(p.Headlines) != 0 {
		t.Fatal("unscoped aggregate labeled as workload headline")
	}

	for _, metric := range p.Metrics {
		if metric.Name == "latency" && !strings.Contains(metric.Charts[0].Scope, "boundaries in ms") {
			t.Fatal("aggregate histogram boundary unit omitted")
		}
	}
}

func TestCatalogOmitsAmbiguousTPSAndShowsNamedDrivers(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	id := "2026-10-13T12-00-00Z-custom"

	data := []byte(`{"kind":"run","schema":3,"workload":"custom","status":"completed",
"started_at":"2026-10-13T12:00:00Z","driver":"noop",
"drivers":[{"name":"default","type":"noop"},{"name":"primary","type":"postgres"}],
"metrics":{"tps":{"type":"gauge","series":[
{"attributes":{"step":"workload","phase":"first"},"total":987},
{"attributes":{"step":"workload","phase":"second"},"total":654}]}}}`)
	if err := os.WriteFile(filepath.Join(directory, id+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	Handler(&report.History{Directory: directory}).ServeHTTP(response,
		httptest.NewRequest(http.MethodGet, "/?driver=postgres", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "primary=postgres") ||
		strings.Contains(response.Body.String(), ">987<") || strings.Contains(response.Body.String(), ">654<") {
		t.Fatalf("misleading catalog summary: %s", response.Body.String())
	}
}
