package view

import (
	"fmt"
	"math"
	"strings"
)

const (
	chartLabelWidth      = 190
	chartPlotWidth       = 420
	chartRowHeight       = 34
	maxChartRows         = 64
	chartFooterHeight    = 44
	maxChartLabelRunes   = 28
	shortChartLabelRunes = 25
)

type bar struct {
	Label string
	Value string
	X     float64
	Y     float64
	Width float64
	Tip   string
}

type chart struct {
	Title             string
	Scope             string
	Unit              string
	Bars              []bar
	Height            int
	Max               string
	HiddenZeroBuckets int
}

type chartValue struct {
	label string
	value float64
	text  string
}

func bars(title, scope, unit string, values []chartValue) chart {
	c := chart{Title: title, Scope: scope, Unit: unit}
	if len(values) == 0 || len(values) > maxChartRows {
		return c
	}

	maximum := 0.0

	for _, value := range values {
		if value.value < 0 || math.IsNaN(value.value) || math.IsInf(value.value, 0) {
			return c
		}

		maximum = max(maximum, value.value)
	}

	if maximum == 0 {
		return c
	}

	c.Max = compact(maximum)

	c.Height = len(values)*chartRowHeight + chartFooterHeight
	for i, value := range values {
		label := value.label
		if len([]rune(label)) > maxChartLabelRunes {
			label = string([]rune(label)[:shortChartLabelRunes]) + "…"
		}

		c.Bars = append(c.Bars, bar{
			Label: label, Value: value.text, X: chartLabelWidth, Y: float64((i + 1) * chartRowHeight),
			Width: value.value / maximum * chartPlotWidth, Tip: value.label + ": " + value.text + " " + unit,
		})
	}

	return c
}

func stepDurations(value any) chart {
	items, _ := value.([]any)
	values := []chartValue{}

	for _, item := range items {
		step, _ := item.(map[string]any)
		if value, ok := number(step["duration_seconds"]); ok {
			values = append(values, chartValue{label: text(step["name"]), value: value, text: compact(value)})
		}
	}

	return bars("Step durations", "Recorded wall-clock time; steps may overlap", "seconds", values)
}

func appendHistogram(charts []chart, name, scope string, metric map[string]any) []chart {
	bounds, _ := metric["bounds"].([]any)

	counts, _ := metric["bucket_counts"].([]any)
	if len(counts) != len(bounds)+1 || len(bounds) == 0 {
		return charts
	}

	previous := math.Inf(-1)

	for _, bound := range bounds {
		value, ok := number(bound)
		if !ok || math.IsInf(value, 0) || math.IsNaN(value) || value <= previous {
			return charts
		}

		previous = value
	}

	values := make([]chartValue, 0, len(counts))
	zeroBuckets := 0

	for i, count := range counts {
		n, ok := number(count)
		if !ok {
			return charts
		}

		if n == 0 && i < len(bounds) {
			zeroBuckets++

			continue
		}

		var label string
		if i < len(bounds) {
			label = "≤ " + text(bounds[i])
			if i > 0 {
				label = text(bounds[i-1]) + " < x ≤ " + text(bounds[i])
			}
		} else {
			label = "> " + text(bounds[len(bounds)-1]) + " (overflow)"
		}

		values = append(values, chartValue{label: label, value: n, text: text(count)})
	}

	c := bars(name+" · histogram", scope+" · non-cumulative bucket counts; unequal intervals", "observations", values)

	c.HiddenZeroBuckets = zeroBuckets
	if len(c.Bars) > 0 {
		charts = append(charts, c)
	}

	return charts
}

type dimension struct {
	value   string
	present bool
}

func metricSeriesChart(name, unit string, series []any) (chart, bool) {
	groups := map[[3]dimension][]chartValue{}

	for _, item := range series {
		point, _ := item.(map[string]any)
		attributes, _ := point["attributes"].(map[string]any)

		value, ok := number(point["total"])
		if !ok {
			return chart{}, false
		}

		labels := []string{}
		for _, key := range sortedKeys(attributes) {
			labels = append(labels, key+"="+text(attributes[key]))
		}

		var group [3]dimension

		for i, key := range []string{"step", "phase", "outcome"} {
			value, present := attributes[key]
			if present {
				label, valid := value.(string)
				if !valid {
					return chart{}, false
				}

				group[i] = dimension{value: label, present: true}
			}
		}

		groups[group] = append(groups[group], chartValue{
			label: strings.Join(labels, ", "), value: value, text: text(point["total"]),
		})
	}

	if len(groups) != 1 {
		return chart{}, false
	}

	for scope, values := range groups {
		plot := bars(name+" · series", fmt.Sprintf("Matching step / phase / outcome: %q / %q / %q",
			scope[0].value, scope[1].value, scope[2].value), unit, values)

		return plot, len(plot.Bars) > 0
	}

	return chart{}, false
}

func numericObjectArray(name string, value any) (chart, bool) {
	items, _ := value.([]any)
	if len(items) < 2 || len(items) > maxChartRows {
		return chart{}, false
	}

	first, _ := items[0].(map[string]any)
	labelKey := ""

	for _, key := range []string{"name", "query", "label", "id"} {
		if _, ok := first[key].(string); ok {
			labelKey = key

			break
		}
	}

	if labelKey == "" {
		return chart{}, false
	}

	valueKey := ""

	for _, key := range append([]string{"mix_percent", "count", "duration_seconds"}, sortedKeys(first)...) {
		if _, ok := number(first[key]); ok {
			valueKey = key

			break
		}
	}

	if valueKey == "" {
		return chart{}, false
	}

	values := make([]chartValue, 0, len(items))
	for _, item := range items {
		object, _ := item.(map[string]any)

		n, ok := number(object[valueKey])
		if !ok {
			return chart{}, false
		}

		label, valid := object[labelKey].(string)
		if !valid || label == "" {
			return chart{}, false
		}

		values = append(values, chartValue{label: label, value: n, text: text(object[valueKey])})
	}

	unit := strings.ReplaceAll(valueKey, "_", " ")
	c := bars(fmt.Sprintf("%s · %s", name, valueKey), "Recorded payload values", unit, values)

	return c, len(c.Bars) > 0
}
