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
// explicit-bucket histogram the duration metrics record into: len(testBounds)+1
// bucket counts, where the trailing count is the +Inf overflow bucket.
func binSamples(samples []float64) txObservation {
	buckets := make([]uint64, len(testBounds)+1)

	for _, s := range samples {
		buckets[sort.SearchFloat64s(testBounds, s)]++
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

	if report.MixTolerancePct != mixTolerancePct {
		t.Fatalf("MixTolerancePct = %f, want %f", report.MixTolerancePct, mixTolerancePct)
	}

	if newOrder.MixMinPercent == nil || *newOrder.MixMinPercent != 44.0 {
		t.Fatalf("new_order MixMinPercent = %v, want the effective floor 44.0", newOrder.MixMinPercent)
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

	// A transaction type never measured must not claim a vacuous response pass.
	if report.Transactions[0].ResponsePass != nil {
		t.Fatalf("new_order ResponsePass = %v, want nil for a zero-count transaction", *report.Transactions[0].ResponsePass)
	}
}

func TestComplianceReportMismatchedBuckets(t *testing.T) {
	obs := make([]txObservation, len(complianceTxTable))
	obs[0] = txObservation{count: 5, bounds: []float64{1, 2}, bucketCounts: []uint64{1, 1}} // missing +Inf bucket

	if _, err := complianceReport(obs, reportOptions{paced: true, elapsed: time.Second}); err == nil {
		t.Fatal("complianceReport returned nil error for a histogram missing the +Inf bucket")
	}
}

func TestComplianceReportAcceptsOverflowBucket(t *testing.T) {
	obs := make([]txObservation, len(complianceTxTable))
	obs[0] = txObservation{count: 5, bounds: []float64{1, 2}, bucketCounts: []uint64{1, 2, 3}} // +Inf shape

	report, err := complianceReport(obs, reportOptions{paced: true, elapsed: time.Second})
	if err != nil {
		t.Fatalf("complianceReport rejected a histogram with the +Inf overflow bucket: %v", err)
	}

	if report.Transactions[0].Count != 5 {
		t.Fatalf("new_order Count = %d, want 5", report.Transactions[0].Count)
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

func TestStockLevelCeilingBoundary(t *testing.T) {
	// Stock-Level ceiling is 20000ms. Samples at 15000ms land in the (10000,20000]
	// bucket, whose upper bound must be 20000 (not 30000), so the run passes.
	obs := fullMix([]float64{50, 40, 30, 60, 15000})

	report, err := complianceReport(
		obs,
		reportOptions{workload: "tpcc/tx", paced: true, elapsed: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}

	stock := report.Transactions[4]
	if stock.Name != "stock_level" {
		t.Fatalf("transaction 4 = %s, want stock_level", stock.Name)
	}

	if stock.P90Ms != 20000 {
		t.Fatalf("stock_level p90 = %f, want 20000 (upper bound)", stock.P90Ms)
	}

	if stock.ResponsePass == nil || !*stock.ResponsePass {
		t.Fatal("stock_level ResponsePass = false, want true for p90 within the 20s ceiling")
	}
}

func TestSteadySnapshotIncludesFinalPartialSlot(t *testing.T) {
	start := time.Unix(0, 0)

	tracker := newSteady(start, steadySlotWidth, minSteadySlots+1)
	for _, offset := range []time.Duration{0, 10 * steadySlotWidth, 20 * steadySlotWidth} {
		tracker.record(start.Add(offset))
	}

	elapsed := time.Duration(minSteadySlots)*steadySlotWidth + steadySlotWidth/10
	tracker.record(start.Add(elapsed))

	series := tracker.snapshot(elapsed)
	if len(series.counts) != minSteadySlots+1 {
		t.Fatalf("snapshot slots = %d, want %d", len(series.counts), minSteadySlots+1)
	}

	if series.counts[minSteadySlots] != 1 {
		t.Fatalf("final partial slot count = %d, want 1", series.counts[minSteadySlots])
	}

	if got := computeSteadiness(series); got.Status != "fail" {
		t.Fatalf("Steadiness.Status = %s, want fail with final partial slot included", got.Status)
	}
}

func TestSteadinessPass(t *testing.T) {
	counts := make([]uint64, 600)
	for i := range counts {
		counts[i] = 7 // perfectly steady New-Order rate
	}

	s := computeSteadiness(steadySeries{counts: counts})

	if s.Status != "pass" {
		t.Fatalf("Steadiness.Status = %s, want pass", s.Status)
	}

	if s.CV != 0 {
		t.Fatalf("Steadiness.CV = %f, want 0 for a steady series", s.CV)
	}
}

func TestSteadinessFail(t *testing.T) {
	counts := make([]uint64, 600)
	for i := range 300 {
		counts[i] = 100
	}
	// Second half at zero: a bimodal series with high coefficient of variation.

	s := computeSteadiness(steadySeries{counts: counts})

	if s.Status != "fail" {
		t.Fatalf("Steadiness.Status = %s, want fail", s.Status)
	}
}

func TestSteadinessInsufficient(t *testing.T) {
	s := computeSteadiness(steadySeries{counts: make([]uint64, minSteadySlots-1)})

	if s.Status != "insufficient" {
		t.Fatalf("Steadiness.Status = %s, want insufficient", s.Status)
	}

	if s.Reason == "" {
		t.Fatal("Steadiness.Reason is empty for an insufficient sample")
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
