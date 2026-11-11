//go:build integration && integration_optional

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTPCDSAnswerValidationTiDB(t *testing.T) {
	NewTiDB(t)
	testTPCDSAnswerValidation(t, "tidb", envOr(envTiDBAllURL, defaultTiDBAllURL))
}

func TestTPCDSAnswerValidationYugabyte(t *testing.T) {
	NewYugabyte(t)
	testTPCDSAnswerValidation(t, "yugabyte", envOr(envYugabyteAllURL, defaultYugabyteAllURL))
}

func TestTPCDSAnswerValidationYDB(t *testing.T) {
	NewYDB(t)
	testTPCDSAnswerValidation(t, "ydb", envOr(envYDBAllURL, defaultYDBAllURL))
}

func testTPCDSAnswerValidation(t *testing.T, driver, url string) {
	t.Helper()
	runStroppy(t, time.Minute, "run", "tpcds", "-d", driver, "-D", "url="+url,
		"--scale-factor", "1", "--steps", "drop_schema,create_schema")

	verb := "INSERT"
	if driver == "ydb" {
		verb = "UPSERT"
	}

	var fixture strings.Builder
	fmt.Fprintf(&fixture, "--= time\n%s INTO time_dim(t_time_sk,t_hour,t_minute) VALUES(1,20,30);\n", verb)
	fmt.Fprintf(&fixture, "--= demographics\n%s INTO household_demographics(hd_demo_sk,hd_dep_count) VALUES(1,7);\n", verb)
	fmt.Fprintf(&fixture, "--= store\n%s INTO store(s_store_sk,s_store_name) VALUES(1,'ese');\n", verb)
	fmt.Fprintf(&fixture,
		"--= sales\n%s INTO store_sales(ss_item_sk,ss_ticket_number,ss_sold_time_sk,ss_hdemo_sk,ss_store_sk) VALUES ", verb)

	for i := 1; i <= 870; i++ {
		if i > 1 {
			fixture.WriteByte(',')
		}

		fmt.Fprintf(&fixture, "(%d,%d,1,1,1)", i, i)
	}

	fixture.WriteString(";\n")

	path := filepath.Join(t.TempDir(), "fixture.sql")
	if err := os.WriteFile(path, []byte(fixture.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	runStroppy(t, time.Minute, "run", path, "-d", driver, "-D", "url="+url, "--iterations", "1", "--no-report")

	reportFile := filepath.Join(t.TempDir(), "validation.json")
	output := runStroppy(t, 3*time.Minute,
		"run", "tpcds", "-d", driver, "-D", "url="+url, "--scale-factor", "1",
		"--steps", "validate_answers", "--query-timeout", "20s", "--report-file", reportFile,
	)

	if !strings.Contains(output, "database defect") {
		t.Fatal("missing qualification attribution warning")
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatal(err)
	}

	var report struct {
		Reports []struct {
			Kind string `json:"kind"`
			Data struct {
				Status string `json:"status"`
				Reason string `json:"reason"`
				Totals struct {
					Total   int `json:"total"`
					Diff    int `json:"diff"`
					Error   int `json:"error"`
					Skipped int `json:"skipped"`
				} `json:"totals"`
				Queries []struct {
					Query  string `json:"query"`
					Status string `json:"status"`
				} `json:"queries"`
			} `json:"data"`
		} `json:"workload_reports"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}

	for _, validation := range report.Reports {
		if validation.Kind != "tpcds.validation" {
			continue
		}

		totals := validation.Data.Totals
		if totals.Total != 103 || totals.Error != 0 || totals.Skipped != 0 || totals.Diff == 0 ||
			validation.Data.Status != "failed" || !strings.Contains(validation.Data.Reason, "database defect") {
			t.Fatalf("fixture must execute all queries and retain expected differences: %+v", validation.Data)
		}

		for _, query := range validation.Data.Queries {
			if query.Query == "query_96" && query.Status == "ok" {
				return
			}
		}

		t.Fatal("q96 fixture did not match the shared 870 reference answer")
	}

	t.Fatal("qualification report missing")
}
