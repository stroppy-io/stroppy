package view

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"strings"

	_ "embed"
)

//go:embed charts.js
var chartAdapter string

var chartScript = compatibleChartScript()

func compatibleChartScript() string {
	kit := kitAsset("chart.js")
	for _, patch := range [][2]string{
		{"const _=new De({", "let cfg={"},
		{
			"},Mf(n),d);if(n.length>1)",
			"};o.configure?.(cfg,De);const _=new De(cfg,o.data??Mf(n),d);o.mounted?.(_);if(n.length>1)",
		},
		{
			"const c=()=>{s?.destroy(),l.replaceChildren(),s=Af(l,n,o)};",
			"const c=()=>{try{s?.destroy(),l.replaceChildren(),s=Af(l,n,o)}" +
				"catch(e){s?.destroy();s=void 0;l.replaceChildren();o.onError?.(e);throw e}};",
		},
	} {
		if strings.Count(kit, patch[0]) != 1 {
			panic("incompatible Stroppy kit chart bundle")
		}

		kit = strings.Replace(kit, patch[0], patch[1], 1)
	}

	return "/*\n" + kitAsset("UPLOT-LICENSE.txt") + "*/\n" + kit + "\n" + chartAdapter
}

func embeddedCharts() template.JS {
	return template.JS(chartScript) //nolint:gosec // inspected embedded code, never report data.
}

func chartPolicy() string {
	digest := sha256.Sum256([]byte(chartScript))

	return "script-src 'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'; "
}

// Chart data stays in an escaped HTML attribute, not executable script content.
func chartJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "null"
	}

	return string(data)
}

type interactiveChart struct {
	Kind   string              `json:"kind"`
	Unit   string              `json:"unit"`
	Series []interactiveSeries `json:"series"`
	Labels []string            `json:"labels,omitempty"`
}

type interactiveSeries struct {
	Title    string           `json:"title"`
	Color    int              `json:"color"`
	Pattern  string           `json:"pattern,omitempty"`
	Points   []timelinePoint  `json:"points,omitempty"`
	Values   []float64        `json:"values,omitempty"`
	Markers  []timelineMarker `json:"markers,omitempty"`
	Interval float64          `json:"interval,omitempty"`
	Omitted  int              `json:"omitted,omitempty"`
}

//nolint:gocritic // templates pass chart values, not addressable Go expressions.
func interactiveBars(c chart) string {
	data := interactiveChart{Kind: "bars", Unit: c.Unit}

	series := interactiveSeries{Title: c.Title, Color: 1}
	for _, bar := range c.Bars {
		data.Labels = append(data.Labels, bar.Category)
		series.Values = append(series.Values, bar.Number)
	}

	data.Series = []interactiveSeries{series}

	return chartJSON(data)
}

func interactiveTimeline(t *timeline) string {
	return chartJSON(interactiveChart{Kind: "timeline", Unit: t.Unit, Series: []interactiveSeries{{
		Title: t.Name + " · " + t.Scope, Color: 1, Points: t.Points, Markers: t.Markers,
		Interval: t.Interval, Omitted: t.Omitted,
	}}})
}

func interactiveComparison(t *comparisonTimeline) string {
	data := interactiveChart{Kind: "timeline", Unit: t.Unit}
	for i := range t.Series {
		line := &t.Series[i]
		data.Series = append(data.Series, interactiveSeries{
			Title: line.Label, Color: line.Series, Pattern: line.Pattern, Points: line.Samples, Markers: line.Markers,
			Interval: line.Interval, Omitted: line.Omitted,
		})
	}

	return chartJSON(data)
}
