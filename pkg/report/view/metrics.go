package view

import (
	"encoding/json"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

const (
	previewDecimals          = 3
	previewSignificantDigits = 3
	million                  = 1_000_000
)

func groupMetrics(metrics []metricView) []metricGroup {
	groups := []metricGroup{
		{Name: "Distributions", Distribution: true},
		{Name: "Counters"},
		{Name: "Gauges"},
		{Name: "Other metrics"},
	}
	for i := range metrics {
		index := len(groups) - 1

		switch metrics[i].Type {
		case "histogram":
			index = 0
		case "counter":
			index = 1
		case "gauge":
			index = 2
		}

		groups[index].Metrics = append(groups[index].Metrics, &metrics[i])
	}

	for i := range groups {
		slices.SortFunc(groups[i].Metrics, func(a, b *metricView) int { return naturalCompare(a.Name, b.Name) })
	}

	return slices.DeleteFunc(groups, func(group metricGroup) bool { return len(group.Metrics) == 0 })
}

func metricLabel(name string) string {
	label := strings.ReplaceAll(strings.TrimSuffix(name, "_total"), "_", " ")

	runes := []rune(label)
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
	}

	return string(runes)
}

func metricUnit(value any) string {
	unit, _ := value.(string)

	return strings.NewReplacer("{", "", "}", "").Replace(unit)
}

func metricStats(metric map[string]any, unit string) []metricStat {
	if metric["type"] != "histogram" {
		return []metricStat{previewStat("Value", metric["total"], unit)}
	}

	percentiles, _ := metric["percentiles"].(map[string]any)

	return []metricStat{
		previewStat("Count", metric["count"], ""),
		previewStat("Average", metric["average"], unit),
		previewStat("p99", percentiles["p99"], unit),
	}
}

func previewStat(label string, value any, unit string) metricStat {
	exact := text(value)
	if value == nil {
		unit = ""
	}

	return metricStat{Label: label, Text: previewNumber(value), Exact: exact, Unit: unit}
}

func previewNumber(value any) string {
	n, ok := value.(json.Number)
	if !ok {
		return text(value)
	}

	raw := n.String()

	parsed, err := n.Float64()
	if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return raw
	}

	if math.Abs(parsed) >= million {
		for _, scale := range []struct {
			value  float64
			suffix string
		}{
			{1e12, "T"}, {1e9, "B"}, {1e6, "M"},
		} {
			if math.Abs(parsed) >= scale.value {
				return "≈" + strconv.FormatFloat(parsed/scale.value, 'g', previewSignificantDigits, 64) + scale.suffix
			}
		}
	}

	if !strings.ContainsAny(raw, ".eE") {
		return groupedDigits(raw)
	}

	formatted := strconv.FormatFloat(parsed, 'f', previewDecimals, 64)
	if parsed != 0 && math.Abs(parsed) < .001 {
		formatted = strconv.FormatFloat(parsed, 'g', previewSignificantDigits, 64)
	}

	if strings.Contains(formatted, ".") && !strings.ContainsAny(formatted, "eE") {
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	}

	approximate := ""
	original, originalOK := new(big.Rat).SetString(raw)

	rounded, roundedOK := new(big.Rat).SetString(formatted)
	if !originalOK || !roundedOK || original.Cmp(rounded) != 0 {
		approximate = "≈"
	}

	whole, fraction, decimal := strings.Cut(formatted, ".")

	formatted = groupedDigits(whole)
	if decimal {
		formatted += "." + fraction
	}

	return approximate + formatted
}

func groupedDigits(value string) string {
	if strings.ContainsAny(value, "eE") {
		return value
	}

	sign := ""
	if strings.HasPrefix(value, "-") {
		sign, value = "-", value[1:]
	}

	var parts []string
	for len(value) > previewDecimals {
		parts = append(parts, value[len(value)-previewDecimals:])
		value = value[:len(value)-previewDecimals]
	}

	parts = append(parts, value)
	slices.Reverse(parts)

	return sign + strings.Join(parts, ",")
}

func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		if a[0] >= '0' && a[0] <= '9' && b[0] >= '0' && b[0] <= '9' {
			aDigits := strings.TrimLeftFunc(a, unicode.IsDigit)
			bDigits := strings.TrimLeftFunc(b, unicode.IsDigit)
			aNumber := strings.TrimLeft(a[:len(a)-len(aDigits)], "0")

			bNumber := strings.TrimLeft(b[:len(b)-len(bDigits)], "0")
			if len(aNumber) != len(bNumber) {
				return len(aNumber) - len(bNumber)
			}

			if result := strings.Compare(aNumber, bNumber); result != 0 {
				return result
			}

			a, b = aDigits, bDigits

			continue
		}

		if a[0] != b[0] {
			return int(a[0]) - int(b[0])
		}

		a, b = a[1:], b[1:]
	}

	return strings.Compare(a, b)
}
