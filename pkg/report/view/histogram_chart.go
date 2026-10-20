package view

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/stroppy-io/stroppy/v6/pkg/report/compare"
)

func comparisonGroupKey(row *compare.Row) string {
	return stringJSON([]any{row.Name, row.Type, row.Unit, row.Scope, row.Attributes})
}

func histogramSummary(row *compare.Row) bool {
	return row.Type == "histogram" && slices.Contains([]string{
		"average", "p50", "p90", "p95", "p99", "bucket_p50", "bucket_p90", "bucket_p95", "bucket_p99",
	}, row.Statistic)
}

func comparisonDedicatedPlot(row *compare.Row) bool {
	return comparisonScalarPlot(row) && !histogramSummary(row)
}

func comparisonLegend(result *compare.Result) []chartLegend {
	legend := make([]chartLegend, 0, len(result.Runs))
	for i, run := range result.Runs {
		legend = append(legend, chartLegend{Label: fmt.Sprintf("Run %d · %s", i+1, run.Workload), Series: i + 1})
	}

	return legend
}

const (
	summaryHeight      = 320
	summaryLeft        = 80.0
	summaryWidth       = 600.0
	summaryBottom      = 260.0
	summaryPlotHeight  = 210.0
	summaryMaxBarWidth = 42.0
	summaryGroupFill   = 0.8
	summaryBarGap      = 2.0
)

func histogramSummaryBar(cell *compare.Cell, label, unit string, run int) (bar, bool) {
	if cell.Value == nil {
		return bar{}, false
	}

	value, err := cell.Value.Float64()
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return bar{}, false
	}

	return bar{
		Label: fmt.Sprintf("%s · Run %d", label, run+1), Category: label, Number: value, Series: run + 1,
		Value: previewNumber(*cell.Value), Tip: fmt.Sprintf("%s · Run %d: %s %s", label, run+1, *cell.Value, unit),
	}, true
}

func histogramSummaryChart(result *compare.Result, group *comparisonMetric) chart {
	c := chart{
		Title: group.Name + " · distribution summary", Unit: group.Unit, Vertical: true, Height: summaryHeight,
		Scope: group.Scope + " · " + group.Label + " · percentiles are histogram estimates; missing values omitted",
	}
	if group.Type != "histogram" {
		return c
	}

	rows := map[string]*compare.Row{}

	for i := range result.Metrics {
		row := &result.Metrics[i]
		if comparisonGroupKey(row) == group.Key && histogramSummary(row) && comparisonScalarPlot(row) {
			rows[row.Statistic] = row
		}
	}

	maximum := 0.0

	for _, statistic := range []string{"average", "p50", "p90", "p95", "p99"} {
		row := rows[statistic]
		if row == nil {
			row = rows["bucket_"+statistic]
		}

		if row == nil {
			continue
		}

		label := statistic
		switch statistic {
		case "average":
			label = "Average"
		case "p50":
			label = "Median / p50"
		}

		category := len(c.Categories)
		c.Categories = append(c.Categories, chartCategory{Label: label})

		for i := range row.Cells {
			b, ok := histogramSummaryBar(&row.Cells[i], label, group.Unit, i)
			if !ok {
				continue
			}

			maximum = max(maximum, b.Number)
			b.X = float64(category)
			c.Bars = append(c.Bars, b)
		}
	}

	if len(c.Categories) == 0 || len(c.Bars) == 0 {
		return c
	}

	c.Legend = comparisonLegend(result)
	c.Max = previewNumber(json.Number(compact(maximum)))

	projectHistogramSummary(&c, len(result.Runs), maximum)

	return c
}

func projectHistogramSummary(c *chart, runs int, maximum float64) {
	slot := summaryWidth / float64(len(c.Categories))

	barWidth := min(summaryMaxBarWidth, slot*summaryGroupFill/float64(runs))
	for i := range c.Categories {
		c.Categories[i].X = summaryLeft + (float64(i)*slot + slot/2)
	}

	for i := range c.Bars {
		b := &c.Bars[i]
		center := c.Categories[int(b.X)].X
		b.X = center - barWidth*float64(runs)/2 + float64(b.Series-1)*barWidth + 1

		b.Width = max(1, barWidth-summaryBarGap)
		if maximum > 0 {
			b.Height = b.Number / maximum * summaryPlotHeight
		}

		b.Y = summaryBottom - b.Height
	}
}
