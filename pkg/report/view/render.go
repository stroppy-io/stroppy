package view

import (
	"encoding/base64"
	"encoding/json"
	"html/template"
	"io"
	"net/url"

	"github.com/stroppy-io/stroppy/v6/pkg/report"

	_ "embed"
)

//go:embed report.html
var reportTemplate string

//go:embed comparison.html
var comparisonTemplate string

//go:embed style.css
var stylesheet string

var templates = template.Must(template.New("report").Funcs(template.FuncMap{
	"statuses": func() []string {
		return []string{"completed", "completed_with_errors", "failed", "canceled"}
	},
	"previousPage":          func(page int) int { return max(0, page-1) },
	"nextPage":              func(page int) int { return page + 1 },
	"style":                 embeddedStyle,
	"kitMark":               kitMark,
	"statusTone":            statusTone,
	"preview":               func(value string) string { return previewNumber(json.Number(value)) },
	"download":              jsonDownload,
	"chartRuntime":          embeddedCharts,
	"interactiveBars":       interactiveBars,
	"interactiveTimeline":   interactiveTimeline,
	"interactiveComparison": interactiveComparison,
	"selected": func(selection []catalogSelection, id string) bool {
		for _, item := range selection {
			if item.ID == id {
				return true
			}
		}

		return false
	},
}).Parse(reportTemplate + comparisonTemplate))

var reportStyles = template.CSS(kitStyle()) //nolint:gosec // inspected embedded styles and fonts, never report data.

func embeddedStyle() template.CSS {
	return reportStyles
}

func jsonDownload(data string) template.URL {
	encoded := base64.StdEncoding.EncodeToString([]byte(data))

	return template.URL("data:application/json;base64," + encoded) //nolint:gosec // fixed MIME and base64 alphabet.
}

// Render writes self-contained HTML with static charts and optional enhancement. Original JSON remains
// available without decoding and re-encoding workload-owned or unknown fields.
func Render(output io.Writer, data []byte, id string) error {
	return RenderCapture(output, data, id, ".", nil)
}

// RenderCapture includes bounded recording views while retaining complete artifact references.
func RenderCapture(output io.Writer, data []byte, id, base string, query url.Values) error {
	p, err := reportPage(data, id)
	if err != nil {
		return err
	}

	if capture, err := report.CaptureFromJSON(data); err == nil && capture != nil {
		loaded := loadCapture(base, capture, query)
		p.Capture = &loaded
	}

	return templates.ExecuteTemplate(output, "report", p)
}

func renderServed(output io.Writer, data []byte, id, download, base string, query url.Values) error {
	p, err := reportPage(data, id)
	if err != nil {
		return err
	}

	p.Catalog = "/"
	p.Download = download

	if capture, err := report.CaptureFromJSON(data); err == nil && capture != nil {
		loaded := loadCapture(base, capture, query)
		loaded.ArtifactBase = "/reports/" + id + "/artifacts/"
		p.Capture = &loaded
	}

	return templates.ExecuteTemplate(output, "report", p)
}
