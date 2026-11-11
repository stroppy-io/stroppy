package tpcds

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestValidationReportUsesPublicStatuses(t *testing.T) {
	report := newValidationReport([]compareResult{
		{query: "q1", status: "ok"},
		{query: "q2", status: "diff", deltas: []string{"different"}},
		{query: "q3", status: "skip", reason: "missing"},
		{query: "q4", status: "error", errMsg: "failed"},
	})

	if report.Status != "failed" || report.Totals.OK != 1 || report.Totals.Diff != 1 ||
		report.Totals.Skipped != 1 || report.Totals.Error != 1 {
		t.Fatalf("validation report = %#v", report)
	}

	for index, status := range []string{"ok", "diff", "skip", "error"} {
		if report.Queries[index].Status != status {
			t.Fatalf("query %d status = %q, want %q", index, report.Queries[index].Status, status)
		}
	}

	if report.Queries[2].Reason != "missing" || len(report.Queries[2].Mismatches) != 0 {
		t.Fatalf("skipped query = %#v", report.Queries[2])
	}
}

func TestCompatibleDriversDeclareAnswerValidation(t *testing.T) {
	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, driver := range []bench.DriverTypeName{
		bench.DriverPostgres, bench.DriverMySQL, bench.DriverTiDB, bench.DriverYugabyte, bench.DriverYDB,
	} {
		description, err := catalog.ResolveRun(Test.Name, bench.RunOptions{
			Drivers: map[string]bench.DriverConfig{"default": {Kind: driver}},
		})
		if err != nil {
			t.Fatalf("%s: %v", driver, err)
		}

		found := false
		for _, step := range description.Steps {
			found = found || step.Name == "validate_answers"
		}

		if !found {
			t.Errorf("%s missing shared qualification step", driver)
		}
	}

	if answerValidationSupported(bench.DriverPicodata) || answerValidationSupported(bench.DriverCRDB) {
		t.Fatal("unfinished SQL ports must remain blocked")
	}

	if validationIsolation(bench.DriverYDB) != bench.IsoConn ||
		validationIsolation(bench.DriverTiDB) != bench.IsoReadCommitted {
		t.Fatal("validation execution policy does not match backend")
	}
}

func TestGeneratedStreamsDoNotCompareQualificationAnswers(t *testing.T) {
	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, driver := range []bench.DriverTypeName{bench.DriverTiDB, bench.DriverYugabyte} {
		for _, params := range []map[string]string{{"query-stream": "0"}, {"streams": "2"}} {
			description, err := catalog.ResolveRun(Test.Name, bench.RunOptions{
				Params:  bench.ParamInputs{CLI: params},
				Drivers: map[string]bench.DriverConfig{"default": {Kind: driver}},
			})
			if err != nil {
				t.Fatal(err)
			}

			for _, step := range description.Steps {
				if step.Name == "validate_answers" {
					t.Fatalf("%s generated stream must not use fixed qualification answers", driver)
				}
			}
		}
	}
}

func TestQualificationMismatchWarning(t *testing.T) {
	results := []compareResult{{query: "query_96", status: "diff", deltas: []string{"different"}}}

	report := newValidationReport(results)
	if report.Status != "failed" || report.Reason != qualificationWarning || report.Totals.Diff != 1 {
		t.Fatalf("mismatch was hidden or attribution warning missing: %+v", report)
	}

	var output bytes.Buffer
	logSummary(&bench.Bench{Log: bench.NewLogger(&output)}, results)

	if !strings.Contains(output.String(), "database defect") || !strings.Contains(output.String(), `"level":"warn"`) {
		t.Fatalf("missing visible mismatch warning: %s", output.String())
	}
}

func TestAllSkippedValidationIsSkipped(t *testing.T) {
	report := newValidationReport([]compareResult{{query: "q1", status: "skip", reason: "missing"}})
	if report.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", report.Status)
	}
}
