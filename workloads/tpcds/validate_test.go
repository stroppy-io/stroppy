package tpcds

import "testing"

func TestValidationReportUsesPublicStatuses(t *testing.T) {
	report := newValidationReport([]compareResult{
		{query: "q1", status: "ok"},
		{query: "q2", status: "diff", deltas: []string{"different"}},
		{query: "q3", status: "skip", deltas: []string{"missing"}},
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
}
