//go:build integration && integration_sf1

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTPCDSQualificationSF1(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	reportFile := filepath.Join(t.TempDir(), "qualification.json")
	runStroppy(t, 60*time.Minute,
		"run", "tpcds", "-d", "pg", "-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL),
		"--scale-factor", "1", "--load-workers", "4",
		"--steps", "drop_schema,create_schema,load_data,create_indexes,analyze,validate_answers",
		"--report-file", reportFile,
	)

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatal(err)
	}

	var report struct {
		WorkloadReports []struct {
			Kind string `json:"kind"`
			Data struct {
				Status string `json:"status"`
				Totals struct {
					Total   int `json:"total"`
					OK      int `json:"ok"`
					Diff    int `json:"diff"`
					Skipped int `json:"skipped"`
					Error   int `json:"error"`
				} `json:"totals"`
			} `json:"data"`
		} `json:"workload_reports"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}

	for _, validation := range report.WorkloadReports {
		if validation.Kind != "tpcds.validation" {
			continue
		}

		totals := validation.Data.Totals
		if validation.Data.Status != "ok" || totals.Total != 103 || totals.OK != 103 ||
			totals.Diff != 0 || totals.Skipped != 0 || totals.Error != 0 {
			t.Fatalf("qualification must pass all 103 kit-backed statements: %+v", validation.Data)
		}

		return
	}

	t.Fatal("TPC-DS qualification report missing")
}
