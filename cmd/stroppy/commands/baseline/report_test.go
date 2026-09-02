package baseline

import (
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func resourceMetrics(metrics ...metricdata.Metrics) metricdata.ResourceMetrics {
	return metricdata.ResourceMetrics{
		ScopeMetrics: []metricdata.ScopeMetrics{{Metrics: metrics}},
	}
}

func counterMetric(name string, value float64) metricdata.Metrics {
	return metricdata.Metrics{
		Name: metricsPrefix + name,
		Data: metricdata.Sum[float64]{
			DataPoints: []metricdata.DataPoint[float64]{{Value: value}},
		},
	}
}

func histogramMetric(name string, count uint64, sum float64, bounds []float64, buckets []uint64) metricdata.Metrics {
	return metricdata.Metrics{
		Name: metricsPrefix + name,
		Data: metricdata.Histogram[float64]{
			DataPoints: []metricdata.HistogramDataPoint[float64]{{
				Count:        count,
				Sum:          sum,
				Bounds:       bounds,
				BucketCounts: buckets,
			}},
		},
	}
}

func TestExtractMetrics(t *testing.T) {
	data := resourceMetrics(
		counterMetric("iterations_total", 42),
		counterMetric("failed_iterations_total", 2),
		counterMetric("insert_rows_total", 1000),
		histogramMetric("insert_duration", 1, 5, []float64{1, 10}, []uint64{0, 1, 0}),
		histogramMetric("iteration_duration", 42, 21, []float64{0.1, 1}, []uint64{10, 30, 2}),
		metricdata.Metrics{Name: "unrelated", Data: metricdata.Sum[float64]{}},
	)

	got := extractMetrics(data)

	if got.iterations != 42 || got.failed != 2 || got.insertRows != 1000 {
		t.Fatalf("extractMetrics() = %+v, want iterations 42, failed 2, rows 1000", got)
	}

	if got.insertDurMs != 5 {
		t.Fatalf("insertDurMs = %v, want 5", got.insertDurMs)
	}

	if got.iter.count != 42 || got.iter.avgMs() != 0.5 {
		t.Fatalf("iter histogram = %+v, want count 42 avg 0.5", got.iter)
	}
}

func TestHistogramQuantile(t *testing.T) {
	h := histogram{
		count:   100,
		bounds:  []float64{0.1, 1, 10},
		buckets: []uint64{50, 45, 4, 1},
	}

	if got := h.quantile(0.5); got != 0.1 {
		t.Fatalf("p50 = %v, want bucket bound 0.1", got)
	}

	if got := h.quantile(0.99); got != 10 {
		t.Fatalf("p99 = %v, want bucket bound 10", got)
	}

	empty := histogram{}
	if got := empty.quantile(0.5); got != 0 {
		t.Fatalf("empty quantile = %v, want 0", got)
	}
}

func tierFixture(name string) TierResult {
	return TierResult{
		Name:        name,
		ParallelVUs: 8,
		TxSingle:    TxStat{TxPerSec: 1000},
		TxParallel:  TxStat{TxPerSec: 5600, P50Ms: 0.5, P99Ms: 1.0},
		Load:        LoadStat{RowsPerSec: 100},
	}
}

func verdictStatus(verdicts []Verdict, check string) (string, bool) {
	for _, verdict := range verdicts {
		if verdict.Check == check {
			return verdict.Status, true
		}
	}

	return "", false
}

func TestEvaluateHealthyTiers(t *testing.T) {
	noop := tierFixture(tierNoop)
	wire := tierFixture(tierWire)
	wire.TxSingle.TxPerSec = 150
	wire.TxParallel.TxPerSec = 800 // below noop, scaling 800/(150*8)=67%: both intact
	wire.TxParallel.P99Ms = 0.5

	verdicts := evaluate([]TierResult{noop, wire})

	for _, check := range []string{
		tierNoop + " errors", tierWire + " errors",
		tierNoop + " vu scaling", tierWire + " vu scaling",
		"loopback latency floor",
	} {
		status, found := verdictStatus(verdicts, check)
		if !found || status != statusOK {
			t.Fatalf("check %q = %q (found %v), want ok", check, status, found)
		}
	}
}

func TestEvaluateWarns(t *testing.T) {
	noop := tierFixture(tierNoop)
	noop.TxSingle.Failed = 3
	noop.TxParallel.TxPerSec = 1000 // scaling 12.5%: far below the 60% floor

	wire := tierFixture(tierWire)
	wire.TxParallel.TxPerSec = 2000 // outperforms collapsed noop: ordering warn
	wire.TxParallel.P50Ms = 0.1
	wire.TxParallel.P99Ms = 2.5 // floor + noise warnings

	verdicts := evaluate([]TierResult{noop, wire})

	for _, check := range []string{
		tierNoop + " errors",
		tierNoop + " vu scaling",
		"loopback latency floor",
		tierWire + " latency noise",
		"tier ordering",
	} {
		status, found := verdictStatus(verdicts, check)
		if !found || status != statusWarn {
			t.Fatalf("check %q = %q (found %v), want warn", check, status, found)
		}
	}
}

func TestEvaluateSkipsScalingAtOneVU(t *testing.T) {
	tier := tierFixture(tierNoop)
	tier.ParallelVUs = 1

	verdicts := evaluate([]TierResult{tier})

	if _, found := verdictStatus(verdicts, tierNoop+" vu scaling"); found {
		t.Fatal("scaling verdict emitted for a single parallel VU")
	}
}

func TestSaveAndLoadPrevious(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	report := &Report{
		Schema:  reportSchema,
		Stroppy: "test",
		Time:    time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		Tiers:   []TierResult{tierFixture(tierNoop)},
	}

	path, err := saveReport(report)
	if err != nil {
		t.Fatalf("saveReport() error = %v", err)
	}

	if path == "" {
		t.Fatal("saveReport() returned an empty path")
	}

	previous, err := loadPrevious(report.Time.Add(time.Minute))
	if err != nil {
		t.Fatalf("loadPrevious() error = %v", err)
	}

	if previous == nil || previous.Stroppy != "test" || len(previous.Tiers) != 1 {
		t.Fatalf("loadPrevious() = %+v, want the saved report", previous)
	}

	// The current run's own file must not surface as its own previous.
	same, err := loadPrevious(report.Time)
	if err != nil {
		t.Fatalf("loadPrevious() same-instant error = %v", err)
	}

	if same != nil {
		t.Fatalf("loadPrevious() at same instant = %+v, want nil", same)
	}
}

func TestLoadPreviousMissesCleanly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	previous, err := loadPrevious(time.Now())
	if err != nil || previous != nil {
		t.Fatalf("loadPrevious() = %+v, %v; want a clean miss", previous, err)
	}
}

func TestFormatRate(t *testing.T) {
	for _, tc := range []struct {
		value float64
		want  string
	}{
		{950, "950"},
		{4_200, "4.2k"},
		{1_234_567, "1.23M"},
	} {
		if got := formatRate(tc.value); got != tc.want {
			t.Fatalf("formatRate(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestTxStatAndLoadStat(t *testing.T) {
	m := runMetrics{
		iterations:  100,
		failed:      1,
		insertRows:  500,
		insertDurMs: 250,
		iter: histogram{
			count:   100,
			sumMs:   50,
			bounds:  []float64{0.1, 1},
			buckets: []uint64{60, 35, 5},
		},
	}

	tx := txStat(&m, 2*time.Second)
	if tx.TxPerSec != 50 || tx.AvgMs != 0.5 || tx.Iterations != 100 || tx.Failed != 1 {
		t.Fatalf("txStat() = %+v", tx)
	}

	if zero := txStat(&m, 0); zero.TxPerSec != 0 {
		t.Fatalf("txStat() with zero duration = %+v, want no division", zero)
	}

	load := loadStat(&m)
	if load.Rows != 500 || load.RowsPerSec != 2000 || load.DurationMs != 250 {
		t.Fatalf("loadStat() = %+v", load)
	}
}
