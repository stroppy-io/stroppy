package view

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/report/compare"
)

func TestHistogramSummaryGroupsOnlySameUnitStatistics(t *testing.T) {
	t.Parallel()

	value := func(number string) compare.Cell {
		n := json.Number(number)

		return compare.Cell{Value: &n}
	}
	result := &compare.Result{Runs: []compare.Run{{Workload: "custom"}, {Workload: "custom"}}}

	for _, statistic := range []string{"average", "p50", "p90", "p95", "p99", "bucket_p99", "count", "sum"} {
		unit := "ms"
		if statistic == "count" {
			unit = "observations"
		}

		cells := []compare.Cell{value("10"), value("20")}
		if statistic == "p99" {
			cells[0] = compare.Cell{Reason: "unavailable"}
		}

		result.Metrics = append(result.Metrics, compare.Row{
			Name: "latency", Type: "histogram", Unit: "ms", ValueUnit: unit, Scope: "series",
			Attributes: map[string]any{"step": "workload"}, Statistic: statistic, Cells: cells,
		})
	}

	groups := comparisonMetrics(result)
	if len(groups) != 1 || len(groups[0].Charts) != 3 {
		t.Fatalf("summary, count, and sum not kept separate: %+v", groups)
	}

	c := &groups[0].Charts[0]
	if !c.Vertical || c.Unit != "ms" || len(c.Categories) != 5 || len(c.Bars) != 9 {
		t.Fatalf("histogram summary has wrong scope or statistics: %+v", c)
	}

	if c.Categories[0].Label != "Average" || c.Categories[1].Label != "Median / p50" ||
		c.Categories[4].Label != "p99" {
		t.Fatal("summary statistic ordering changed")
	}

	checkHistogramSummaryRendering(t, c)
}

func checkHistogramSummaryRendering(t *testing.T, c *chart) {
	t.Helper()

	var data interactiveChart
	if err := json.Unmarshal([]byte(interactiveBars(*c)), &data); err != nil {
		t.Fatal(err)
	}

	if data.Kind != "grouped-bars" || data.Series[0].Color != 1 || data.Series[1].Color != 2 ||
		data.Series[0].BarValues[4] != nil || *data.Series[1].BarValues[4] != 20 {
		t.Fatalf("run colors or missing values lost: %+v", data)
	}

	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "chart", c); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), `data-series="2"`) || !strings.Contains(output.String(), "Median / p50") {
		t.Fatal("static grouped chart lost categories or colors")
	}
}

func TestComparisonBarsKeepColorWhenAnotherRunIsMissing(t *testing.T) {
	t.Parallel()

	number := json.Number("42")
	result := &compare.Result{
		Runs: []compare.Run{{Workload: "custom"}, {Workload: "custom"}},
		Metrics: []compare.Row{{
			Name: "tps", Type: "gauge", Unit: "tx/s", ValueUnit: "tx/s", Scope: "series",
			Statistic: "total", Cells: []compare.Cell{{Reason: "missing"}, {Value: &number}},
		}},
	}

	c := comparisonMetrics(result)[0].Charts[0]
	if len(c.Bars) != 1 || c.Bars[0].Series != 2 {
		t.Fatal("missing run repainted remaining run")
	}

	var data interactiveChart
	if err := json.Unmarshal([]byte(interactiveBars(c)), &data); err != nil {
		t.Fatal(err)
	}

	if data.Series[0].BarValues[0] != nil || data.Series[1].Color != 2 || *data.Series[1].BarValues[0] != 42 {
		t.Fatal("interactive bar replaced missing value with zero or changed color")
	}
}
