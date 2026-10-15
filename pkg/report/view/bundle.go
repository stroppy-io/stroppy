package view

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// Bundle exports a readable offline HTML preview and every original capture artifact.
func Bundle(output io.Writer, history *report.History, id string) error {
	data, err := history.Get(id)
	if err != nil {
		return err
	}

	return BundleData(output, data, id, history.Directory)
}

// BundleData creates portable report JSON, HTML, and complete recording streams.
func BundleData(output io.Writer, data []byte, id, base string) error {
	portable, err := report.PortableCaptureJSON(data)
	if err != nil {
		return err
	}

	p, err := reportPage(portable, id)
	if err != nil {
		return err
	}

	if c, err := report.CaptureFromJSON(data); err == nil && c != nil {
		loaded := loadCapture(base, c, nil)
		loaded.ArtifactBase = "captures/" + c.ID + "/"
		p.Capture = &loaded
	}

	var html bytes.Buffer
	if err := templates.ExecuteTemplate(&html, "report", p); err != nil {
		return err
	}

	writer := zip.NewWriter(output)

	entry, err := writer.Create("index.html")
	if err != nil {
		return errors.Join(err, writer.Close())
	}

	if _, err := html.WriteTo(entry); err != nil {
		return errors.Join(err, writer.Close())
	}

	if err := report.WriteBundleInto(writer, data, base); err != nil {
		return errors.Join(err, writer.Close())
	}

	return writer.Close()
}
