package tpch

import "testing"

func TestCompareQueryBoundsMismatchDetails(t *testing.T) {
	got := make([][]any, maxDeltas+10)
	want := answerBlock{Rows: make([][]string, maxDeltas+10)}

	for idx := range got {
		got[idx] = []any{"got"}
		want.Rows[idx] = []string{"want"}
	}

	result := compareQuery("q1", got, want)
	if result.status != "diff" {
		t.Fatalf("status = %q, want diff", result.status)
	}

	if len(result.deltas) != maxDeltas {
		t.Fatalf("deltas = %d, want %d", len(result.deltas), maxDeltas)
	}
}

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
