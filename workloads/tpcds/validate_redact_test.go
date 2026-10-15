package tpcds

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestValidationReportRedactsDiagnosticFields(t *testing.T) {
	t.Parallel()

	password := "private-password"
	w := &workload{validation: validationReport{
		Status: "failed", Reason: password,
		Queries: []validationQuery{{Query: "query_1", Status: "error", Error: password}},
	}}
	test := bench.Test{Name: "validation-redaction", Define: func(d *bench.Def) error {
		d.Drivers.Declare("default", bench.DriverConfig{Kind: bench.DriverNoop, AuthPassword: &password})
		d.Report.Contribute("tpcds.validation", 1, w.validationContribution)
		d.Execution.Step("workload", func(context.Context, *bench.Bench) error { return nil })

		return d.Execution.Err()
	}}

	run, err := bench.RunTest(t.Context(), test, bench.RunOptions{Report: &bench.ReportOptions{}})
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(run.WorkloadReports)
	if err != nil || strings.Contains(string(data), password) || !strings.Contains(string(data), "redacted") {
		t.Fatalf("validation diagnostics = %s, %v", data, err)
	}
}
