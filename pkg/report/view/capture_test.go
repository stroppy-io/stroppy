package view

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestCaptureViewerBoundsLogsAndPreservesGaps(t *testing.T) {
	t.Parallel()

	id := uuid.NewString()

	directory := filepath.Join(t.TempDir(), id)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}

	c := &report.Capture{
		Schema: 1, ID: id, Directory: directory, State: "complete", IntervalSeconds: 1,
		Streams: map[string]report.CaptureStream{
			"logs":    {File: "logs.jsonl", Enabled: true},
			"metrics": {File: "metrics.jsonl", Enabled: true},
			"events":  {File: "events.jsonl", Enabled: true},
		},
	}
	write := func(name string, data []byte) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	encode := func(value any) []byte {
		t.Helper()

		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}

		return data
	}
	write("manifest.json", encode(c))

	log := map[string]any{
		"elapsed_seconds": 1, "level": "info", "message": "<script>raw</script>",
		"fields": map[string]string{"step": "workload"},
	}
	write("logs.jsonl", append(append(encode(log), '\n'), []byte(`{"broken":`)...))
	write("events.jsonl", []byte(`{"elapsed_seconds":0,"kind":"step_start","step":"workload"}`+"\n"))

	var stream bytes.Buffer

	for _, sample := range []struct {
		sequence       int
		elapsed, total float64
	}{{1, 0, 0}, {2, 1, 10}, {4, 3, 12}} {
		data := encode(map[string]any{
			"sequence": sample.sequence, "elapsed_seconds": sample.elapsed,
			"metrics": map[string]any{"tps": map[string]any{
				"unit": "tx/s", "series": []any{map[string]any{
					"attributes": map[string]string{"step": "workload"}, "total": sample.total,
				}},
			}},
		})
		stream.Write(data)
		stream.WriteByte('\n')
	}

	write("metrics.jsonl", stream.Bytes())

	loaded := loadCapture(".", c, nil)
	if len(loaded.Logs) != 1 || !strings.Contains(loaded.Notice, "Incomplete") || len(loaded.Timeline) != 1 {
		t.Fatalf("recording view: %+v", loaded)
	}

	if len(loaded.Timeline[0].Segments) != 2 || len(loaded.Timeline[0].Markers) != 1 {
		t.Fatal("gap or phase boundary hidden")
	}

	document := encode(map[string]any{"kind": "run", "capture": c, "workload": "custom"})

	var output bytes.Buffer
	if err := Render(&output, document, ""); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(output.String(), "<script>raw</script>") ||
		!strings.Contains(output.String(), "Recorded run evolution") {
		t.Fatal("rich HTML unsafe or missing")
	}
}
