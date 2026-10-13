package view

import (
	"encoding/base64"
	"html/template"
	"io"

	_ "embed"
)

//go:embed report.html
var reportTemplate string

//go:embed style.css
var stylesheet string

var templates = template.Must(template.New("report").Funcs(template.FuncMap{
	"statuses": func() []string { return []string{"completed", "completed_with_errors", "failed", "canceled"} },
	"style":    embeddedStyle,
	"download": jsonDownload,
}).Parse(reportTemplate))

func embeddedStyle() template.CSS {
	return template.CSS(stylesheet) //nolint:gosec // embedded application CSS, not report data.
}

func jsonDownload(data string) template.URL {
	encoded := base64.StdEncoding.EncodeToString([]byte(data))

	return template.URL("data:application/json;base64," + encoded) //nolint:gosec // fixed MIME and base64 alphabet.
}

// Render writes a self-contained, script-free HTML report. Original JSON remains
// available without decoding and re-encoding workload-owned or unknown fields.
func Render(output io.Writer, data []byte, id string) error {
	p, err := reportPage(data, id)
	if err != nil {
		return err
	}

	return templates.ExecuteTemplate(output, "report", p)
}

func renderServed(output io.Writer, data []byte, id, download string) error {
	p, err := reportPage(data, id)
	if err != nil {
		return err
	}

	p.Catalog = "/"
	p.Download = download

	return templates.ExecuteTemplate(output, "report", p)
}
