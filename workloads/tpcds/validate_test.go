package tpcds

import "testing"

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

func TestAllSkippedValidationIsSkipped(t *testing.T) {
	report := newValidationReport([]compareResult{{query: "q1", status: "skip", reason: "missing"}})
	if report.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", report.Status)
	}
}
