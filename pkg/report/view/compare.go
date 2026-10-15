package view

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/pkg/report/compare"
)

type comparisonPage struct {
	Result          *compare.Result
	Differences     []comparisonDifference
	Groups          []comparisonMetric
	Highlights      []*comparisonMetric
	Runs            []comparisonRun
	Original        string
	Query           url.Values
	Served          bool
	Timeline        []comparisonTimeline
	MetricNames     []string
	TimelineNotices []string
}
type comparisonDifference struct {
	Path   string
	Values []node
}
type comparisonMetric struct {
	Name, Type, Unit, Scope, Key, Label string
	Attributes                          node
	Rows                                []comparisonRow
	Charts                              []chart
}
type comparisonRow struct {
	Statistic string
	Cells     []comparisonCell
}
type (
	comparisonCell struct{ Value, Delta, Percent, Reason, Exact, ExactDelta, ExactPercent string }
	comparisonRun  struct {
		ID, Label, Workload, Status, Source string
		Warnings                            []string
	}
)

type comparisonTimeline struct {
	Name, Scope, Unit, MetricType, Elapsed, Maximum, Minimum string
	Series                                                   []comparisonLine
}
type comparisonLine struct {
	Label, Pattern, URL string
	Samples             []timelinePoint
	Markers             []timelineMarker
	Segments            []string
	Omitted             int
	Interval            float64
}

// RenderComparison uses the common calculation result for served and offline HTML.
func RenderComparison(output io.Writer, result *compare.Result, base string, query url.Values, served bool) error {
	if query == nil {
		query = url.Values{}
	}

	p := comparisonPage{Result: result, Query: query, Served: served}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	p.Original = string(data)
	for i, run := range result.Runs {
		p.Runs = append(p.Runs, comparisonRun{
			ID: run.ID, Label: fmt.Sprintf("Run %d", i+1),
			Workload: run.Workload, Status: run.Status, Source: string(run.Source), Warnings: run.Warnings,
		})
	}

	for _, difference := range result.Differences {
		row := comparisonDifference{Path: difference.Path}
		for _, value := range difference.Values {
			if value.Present {
				row.Values = append(row.Values, tree(value.Data))
			} else {
				row.Values = append(row.Values, node{Text: "unavailable"})
			}
		}

		p.Differences = append(p.Differences, row)
	}

	p.Groups = comparisonMetrics(result)
	for index := range p.Groups {
		group := &p.Groups[index]
		if slices.Contains([]string{"tps", "queries_per_second", "iterations_per_second"}, group.Name) &&
			group.Label == "step=workload" && group.Scope == "series" {
			p.Highlights = append(p.Highlights, group)
		}
	}

	p.Timeline, p.MetricNames, p.TimelineNotices = comparisonTimelines(result, base, query)

	return templates.ExecuteTemplate(output, "comparison", p)
}

func comparisonMetrics(result *compare.Result) []comparisonMetric {
	groups := []comparisonMetric{}

	for rowIndex := range result.Metrics {
		row := &result.Metrics[rowIndex]
		key := stringJSON([]any{row.Name, row.Type, row.Unit, row.Scope, row.Attributes})

		index := slices.IndexFunc(groups, func(g comparisonMetric) bool { return g.Key == key })
		if index < 0 {
			groups = append(groups, comparisonMetric{
				Name: row.Name, Type: row.Type, Unit: row.Unit, Scope: row.Scope, Key: key,
				Attributes: tree(row.Attributes), Label: comparisonScopeLabel(row.Attributes),
			})
			index = len(groups) - 1
		}

		projected := comparisonRow{Statistic: comparisonStatistic(row)}
		values := []chartValue{}

		for i := range row.Cells {
			cell := &row.Cells[i]

			shown := projectComparisonCell(cell)
			if cell.Value != nil {
				v, err := cell.Value.Float64()
				if err == nil {
					values = append(values, chartValue{
						label: fmt.Sprintf("Run %d · %s", i+1, result.Runs[i].Workload), value: v, text: shown.Value,
					})
				}
			}

			projected.Cells = append(projected.Cells, shown)
		}

		groups[index].Rows = append(groups[index].Rows, projected)
		if comparisonScalarPlot(row) {
			c := bars(row.Name+" · "+projected.Statistic, row.Scope+" "+stringJSON(row.Attributes), row.ValueUnit, values)
			c.Max = previewNumber(json.Number(c.Max))

			if len(c.Bars) > 0 {
				groups[index].Charts = append(groups[index].Charts, c)
			}
		}
	}

	for i := range groups {
		groups[i].Charts = append(groups[i].Charts, comparisonBucketCharts(result, &groups[i])...)
	}

	return groups
}

func comparisonStatistic(row *compare.Row) string {
	label := row.Statistic
	if row.ValueUnit != "" {
		label += " [" + row.ValueUnit + "]"
	}

	if row.Approximate {
		label += " · approximate histogram estimate"
	}

	return label
}

func comparisonScopeLabel(attributes map[string]any) string {
	labels := make([]string, 0, len(attributes))
	for _, key := range sortedKeys(attributes) {
		labels = append(labels, key+"="+text(attributes[key]))
	}

	if len(labels) == 0 {
		return "no dimensions"
	}

	return strings.Join(labels, ", ")
}

func comparisonScalarPlot(row *compare.Row) bool {
	if strings.HasPrefix(row.Statistic, "bucket:") {
		return false
	}

	return !slices.ContainsFunc(row.Cells, func(cell compare.Cell) bool { return cell.DeltaReason != "" })
}

func projectComparisonCell(cell *compare.Cell) comparisonCell {
	shown := comparisonCell{Value: "—", Delta: "—", Percent: "—", Reason: cell.Reason}
	if cell.Value != nil {
		shown.Value, shown.Exact = previewNumber(*cell.Value), string(*cell.Value)
	}

	if cell.Delta != nil {
		shown.Delta, shown.ExactDelta = previewNumber(*cell.Delta), string(*cell.Delta)
	} else if cell.DeltaReason != "" {
		shown.Delta = cell.DeltaReason
	}

	if cell.Percent != nil {
		shown.Percent, shown.ExactPercent = previewNumber(*cell.Percent)+"%", string(*cell.Percent)+"%"
	} else if cell.PercentReason != "" {
		shown.Percent = cell.PercentReason
	}

	if cell.Rounded {
		shown.Percent = "≈" + strings.TrimPrefix(shown.Percent, "≈")
	}

	return shown
}

func comparisonBucketCharts(result *compare.Result, group *comparisonMetric) []chart {
	layouts := map[string][]chartValue{}

	for rowIndex := range result.Metrics {
		row := &result.Metrics[rowIndex]

		key := stringJSON([]any{row.Name, row.Type, row.Unit, row.Scope, row.Attributes})
		if key != group.Key || !strings.HasPrefix(row.Statistic, "bucket:") {
			continue
		}

		label := comparisonBucketLabel(row)

		layout := stringJSON(row.Bounds)
		for i, cell := range row.Cells {
			if cell.Value == nil {
				continue
			}

			value, err := cell.Value.Float64()
			if err != nil {
				continue
			}

			layouts[layout] = append(layouts[layout], chartValue{
				label: fmt.Sprintf("Run %d · %s", i+1, label), value: value, text: string(*cell.Value),
			})
		}
	}

	keys := make([]string, 0, len(layouts))
	for key := range layouts {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	charts := []chart{}

	for _, layout := range keys {
		c := bars(group.Name+" · bucket counts",
			"Identical bucket boundaries "+layout+" "+group.Unit+"; non-cumulative counts", "observations", layouts[layout])
		if len(c.Bars) > 0 {
			charts = append(charts, c)
		}
	}

	return charts
}

func comparisonBucketLabel(row *compare.Row) string {
	index, _ := strconv.Atoi(strings.TrimPrefix(row.Statistic, "bucket:"))
	if index < len(row.Bounds) {
		label := "≤ " + string(row.Bounds[index])
		if index > 0 {
			label = string(row.Bounds[index-1]) + " < x " + label
		}

		return label
	}

	if len(row.Bounds) > 0 {
		return "> " + string(row.Bounds[len(row.Bounds)-1]) + " (overflow)"
	}

	return "overflow"
}

func stringJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "unavailable: " + err.Error()
	}

	return string(data)
}

func comparisonTimelines(
	result *compare.Result, base string, query url.Values,
) (plots []comparisonTimeline, metricNames, problems []string) {
	type source struct {
		plots    []timeline
		interval float64
	}

	sources := make([]source, len(result.Runs))
	names := map[string]any{}
	notices := []string{}

	for i, run := range result.Runs {
		capture, err := report.CaptureFromJSON(run.Source)
		if err != nil || capture == nil || !capture.Streams["metrics"].Enabled {
			notices = append(notices,
				fmt.Sprintf("Run %d: no captured metrics; final-report comparison remains available.", i+1))

			continue
		}

		loaded := loadCapture(base, capture, url.Values{"metric": {query.Get("metric")}})

		sources[i] = source{plots: loaded.Timeline, interval: capture.IntervalSeconds}
		for _, name := range loaded.MetricNames {
			names[name] = nil
		}

		if loaded.Notice != "" || !loaded.Complete {
			notices = append(notices, fmt.Sprintf("Run %d: capture %s. %s", i+1, loaded.State, loaded.Notice))
		}
	}

	groups := []comparisonTimeline{}

	for i, src := range sources {
		for plotIndex := range src.plots {
			plot := &src.plots[plotIndex]
			key := stringJSON([]string{plot.Name, plot.Scope, plot.Unit, plot.MetricType})

			index := slices.IndexFunc(groups, func(g comparisonTimeline) bool {
				return stringJSON([]string{g.Name, g.Scope, g.Unit, g.MetricType}) == key
			})
			if index < 0 {
				groups = append(groups, comparisonTimeline{
					Name: plot.Name, Scope: plot.Scope, Unit: plot.Unit, MetricType: plot.MetricType,
				})
				index = len(groups) - 1
			}

			line := comparisonLine{
				Label: fmt.Sprintf("Run %d · sampling %g s", i+1, src.interval), Pattern: linePattern(i),
				Samples: plot.Points, Omitted: plot.Omitted,
				Interval: src.interval, URL: "/reports/" + result.Runs[i].ID + "#captured-logs",
			}
			line.Markers = append(line.Markers, plot.Markers...)
			groups[index].Series = append(groups[index].Series, line)
		}
	}

	for index := range groups {
		projectComparisonTimeline(&groups[index])
	}

	return groups, sortedKeys(names), notices
}

func linePattern(i int) string {
	return []string{"none", "8 4", "2 4", "10 3 2 3", "4 3", "12 5", "2 2 8 2", "1 5"}[i%compare.MaxRuns]
}

func projectComparisonTimeline(group *comparisonTimeline) {
	last, low, high := 0.0, 0.0, 0.0

	for lineIndex := range group.Series {
		line := &group.Series[lineIndex]
		for _, point := range line.Samples {
			x, _ := strconv.ParseFloat(point.Time, 64)
			y, _ := strconv.ParseFloat(point.Value, 64)
			last = math.Max(last, x)
			low = math.Min(low, y)
			high = math.Max(high, y)
		}
	}

	if last <= 0 {
		last = 1
	}

	if high == low {
		high = low + 1
	}

	group.Elapsed = previewNumber(json.Number(compact(last)))
	group.Minimum = previewNumber(json.Number(compact(low)))

	group.Maximum = previewNumber(json.Number(compact(high)))
	for i := range group.Series {
		line := &group.Series[i]
		segment := make([]string, 0, len(line.Samples))

		var (
			previous     uint64
			previousTime float64
		)

		for j := range line.Samples {
			point := &line.Samples[j]
			x, _ := strconv.ParseFloat(point.Time, 64)

			y, _ := strconv.ParseFloat(point.Value, 64)
			if j > 0 && (point.Sequence != previous+1 || x < previousTime || x-previousTime > line.Interval*1.8) {
				line.Segments = append(line.Segments, strings.Join(segment, " "))
				segment = nil
			}

			previous, previousTime = point.Sequence, x
			point.X, point.Y = timelineLeft+x/last*timelineWidth, timelineBottom-(y-low)/(high-low)*timelineHeight
			segment = append(segment, fmt.Sprintf("%.2f,%.2f", point.X, point.Y))
		}

		if len(segment) > 0 {
			line.Segments = append(line.Segments, strings.Join(segment, " "))
		}

		if line.Omitted > 0 {
			line.Segments = nil
		}

		for j := range line.Markers {
			line.Markers[j].X = timelineLeft + line.Markers[j].Elapsed/last*timelineWidth
		}
	}
}
