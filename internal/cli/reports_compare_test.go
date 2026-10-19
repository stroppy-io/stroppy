package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/pkg/report/compare"
)

func TestReportComparisonCLISharedValuesAndExport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	history, err := report.DefaultHistory()
	if err != nil {
		t.Fatal(err)
	}

	ids := make([]string, 0, 2)

	for i := range 2 {
		value := float64(i+1) * 10

		path, err := history.Save(&report.Run{
			Schema: report.SchemaVersion, Kind: report.Kind, ID: "reused", Workload: "custom",
			Status: report.StatusCompleted, StartedAt: time.Now(),
			Metrics: map[string]report.Metric{"custom": {Type: "counter", Total: &value}},
			Steps:   []report.Step{{Name: "workload", Status: "completed"}},
		})
		if err != nil {
			t.Fatal(err)
		}

		ids = append(ids, strings.TrimSuffix(filepath.Base(path), ".json"))
	}

	output, _, err := reportCLI(t, "reports", "compare", ids[0], ids[1], "-o", "json")
	if err != nil {
		t.Fatal(err)
	}

	var result compare.Result
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}

	cell := result.Metrics[0].Cells[1]
	if result.Reference != ids[0] || string(*cell.Delta) != "10" || string(*cell.Percent) != "100" {
		t.Fatal("CLI calculations diverged")
	}

	human, _, err := reportCLI(t, "reports", "compare", ids[0], ids[1])
	if err != nil || !strings.Contains(human, "100") {
		t.Fatal("human comparison missing delta")
	}

	path := filepath.Join(t.TempDir(), "comparison.html")
	if _, _, err := reportCLI(t, "reports", "compare", ids[0], ids[1], "-o", "html", "--file", path); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "Selected benchmark runs") ||
		!strings.Contains(string(data), `class="chart-scroll"`) {
		t.Fatal("offline export unavailable")
	}

	source := filepath.Join(history.Directory, ids[0]+".json")
	if _, _, err := reportCLI(t, "reports", "compare", ids[0], ids[1], "-o", "html", "--file", source); err == nil {
		t.Fatal("comparison overwrote source")
	}
}
