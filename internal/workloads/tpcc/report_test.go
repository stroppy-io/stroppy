package tpcc

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"
)

// testBounds is a small deterministic histogram layout used only by the unit
// tests to turn raw latency samples into txObservation values.
var testBounds = []float64{100, 500, 1000, 2000, 5000, 10000, 20000, 30000, 60000}

// binSamples sorts raw millisecond samples into testBounds buckets, mirroring the
// explicit-bucket histogram the duration metrics record into.
func binSamples(samples []float64) txObservation {
	buckets := make([]uint64, len(testBounds))

	for _, s := range samples {
		i := sort.SearchFloat64s(testBounds, s)
		if i >= len(testBounds) {
			i = len(testBounds) - 1
		}

		buckets[i]++
	}

	return txObservation{count: uint64(len(samples)), bounds: testBounds, bucketCounts: buckets}
}

func repeat(v float64, n int) []float64 {
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = v
	}

	return samples
}

func fullMix(latencies []float64) []txObservation {
	return []txObservation{
		binSamples(repeat(latencies[0], 450)),
		binSamples(repeat(latencies[1], 430)),
		binSamples(repeat(latencies[2], 40)),
		binSamples(repeat(latencies[3], 40)),
		binSamples(repeat(latencies[4], 40)),
	}
}

func TestComplianceReportPassingPacedRun(t *testing.T) {
	report, err := complianceReport(
		fullMix([]float64{50, 40, 30, 60, 80}),
		reportOptions{workload: "tpcc/tx", paced: true, elapsed: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !report.ComplianceApplicable {
		t.Fatal("ComplianceApplicable = false, want true for a paced run")
	}

	if report.MixCompliant == nil || !*report.MixCompliant {
		t.Fatal("MixCompliant = false, want true for a 45/43/4/4/4 mix")
	}

	if report.ResponseCompliant == nil || !*report.ResponseCompliant {
		t.Fatal("ResponseCompliant = false, want true for sub-second latencies")
	}

	if !report.Statistical.Sufficient {
		t.Fatalf("Statistical.Sufficient = false, want true; reason=%q", report.Statistical.Reason)
	}

	newOrder := report.Transactions[0]
	if newOrder.Name != "new_order" || newOrder.Count != 450 {
		t.Fatalf("new_order tx = %+v, want name new_order count 450", newOrder)
	}

	if newOrder.MixPercent != 45.0 {
		t.Fatalf("new_order mix = %f, want 45.0", newOrder.MixPercent)
	}

	if newOrder.ResponsePass == nil || !*newOrder.ResponsePass {
		t.Fatal("new_order ResponsePass = false, want true")
	}

	if report.TpmC != 450 {
		t.Fatalf("TpmC = %f, want 450", report.TpmC)
	}
}

func TestComplianceReportFailingResponseTime(t *testing.T) {
	report, err := complianceReport(
		fullMix([]float64{6000, 40, 30, 60, 80}), // New-Order above the 5s ceiling
		reportOptions{workload: "tpcc/tx", paced: true, elapsed: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}

	if report.ResponseCompliant == nil || *report.ResponseCompliant {
		t.Fatal("ResponseCompliant = true, want false when New-Order p90 exceeds 5s")
	}

	if report.Transactions[0].ResponsePass == nil || *report.Transactions[0].ResponsePass {
		t.Fatal("new_order ResponsePass = true, want false")
	}
}

func TestComplianceReportFailingMix(t *testing.T) {
	obs := []txObservation{
		binSamples(nil),             // no New-Orders
		binSamples(repeat(50, 100)), // all Payments
		binSamples(nil),
		binSamples(nil),
		binSamples(nil),
	}

	report, err := complianceReport(
		obs,
		reportOptions{workload: "tpcc/tx", paced: true, elapsed: 10 * time.Second},
	)
	if err != nil {
		t.Fatal(err)
	}

	if report.MixCompliant == nil || *report.MixCompliant {
		t.Fatal("MixCompliant = true, want false when payment dominates the mix")
	}

	if report.Transactions[0].MixPass == nil || *report.Transactions[0].MixPass {
		t.Fatal("new_order MixPass = true, want false for 0%% mix")
	}
}

func TestComplianceReportInsufficientSample(t *testing.T) {
	obs := []txObservation{
		binSamples(repeat(50, 10)), // 10 New-Orders < minNewOrdersForValidity
		binSamples(repeat(40, 10)),
		binSamples(repeat(30, 4)),
		binSamples(repeat(60, 4)),
		binSamples(repeat(80, 4)),
	}

	report, err := complianceReport(
		obs,
		reportOptions{workload: "tpcc/tx", paced: true, elapsed: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}

	if report.Statistical.Sufficient {
		t.Fatal("Statistical.Sufficient = true, want false for a small sample")
	}

	if report.Statistical.Reason == "" {
		t.Fatal("Statistical.Reason is empty, want an insufficient-sample explanation")
	}

	if report.Statistical.NewOrders != 10 {
		t.Fatalf("Statistical.NewOrders = %d, want 10", report.Statistical.NewOrders)
	}
}

func TestComplianceReportUnpacedRun(t *testing.T) {
	report, err := complianceReport(
		fullMix([]float64{50, 40, 30, 60, 80}),
		reportOptions{workload: "tpcc/tx", paced: false, elapsed: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}

	if report.ComplianceApplicable {
		t.Fatal("ComplianceApplicable = true, want false for an unpaced run")
	}

	if report.MixCompliant != nil || report.ResponseCompliant != nil {
		t.Fatal("MixCompliant/ResponseCompliant set, want nil for an unpaced run")
	}

	if report.Note == "" {
		t.Fatal("Note is empty, want a compliance-not-applicable explanation")
	}

	if report.Transactions[0].CeilingMs != nil {
		t.Fatal("CeilingMs set on an unpaced run, want nil")
	}
}

func TestComplianceReportMismatchedBuckets(t *testing.T) {
	obs := []txObservation{
		{bounds: []float64{1, 2}, bucketCounts: []uint64{1}},
		{},
		{},
		{},
		{},
	}

	if _, err := complianceReport(obs, reportOptions{paced: true, elapsed: time.Second}); err == nil {
		t.Fatal("complianceReport returned nil error for mismatched bounds/buckets")
	}
}

func TestObservationQuantile(t *testing.T) {
	obs := binSamples(repeat(6000, 100)) // all samples in the (5000,10000] bucket

	if got := obs.quantile(0.90); got != 10000 {
		t.Fatalf("quantile(0.90) = %f, want 10000", got)
	}

	if got := obs.quantile(0.50); got != 10000 {
		t.Fatalf("quantile(0.50) = %f, want 10000", got)
	}

	if got := (txObservation{}).quantile(0.90); got != 0 {
		t.Fatalf("empty quantile = %f, want 0", got)
	}
}

func TestReportText(t *testing.T) {
	report, err := complianceReport(
		fullMix([]float64{50, 40, 30, 60, 80}),
		reportOptions{workload: "tpcc/tx", paced: true, elapsed: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"=== TPC-C compliance report (tpcc/tx) ===",
		"new_order",
		"PASS",
		"tpmC: 450.00",
		"statistical validity: SUFFICIENT",
	} {
		if !strings.Contains(report.text(), want) {
			t.Fatalf("text() missing %q:\n%s", want, report.text())
		}
	}
}

func TestReportJSONRoundTrip(t *testing.T) {
	report, err := complianceReport(
		fullMix([]float64{50, 40, 30, 60, 80}),
		reportOptions{workload: "tpcc/tx", paced: true, elapsed: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := json.Marshal(map[string]Report{"compliance": report})
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]Report

	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}

	got, ok := decoded["compliance"]
	if !ok {
		t.Fatal(`"compliance" key missing from JSON`)
	}

	if len(got.Transactions) != 5 {
		t.Fatalf("JSON report has %d transactions, want 5", len(got.Transactions))
	}
}
