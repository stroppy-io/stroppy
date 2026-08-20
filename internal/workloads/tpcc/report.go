package tpcc

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

// txType enumerates the five TPC-C transaction types in §5.2.3 mix order.
type txType uint8

const (
	txNewOrder txType = iota
	txPayment
	txOrderStatus
	txDelivery
	txStockLevel
)

// TPC-C compliance constants. Each is a named value with the clause it is drawn
// from, so reviewers can trace the number back to the specification.
const (
	// §5.2.5 response-time ceilings: the 90th percentile of each transaction's
	// response time must not exceed these values, in milliseconds.
	ceilingNewOrderMillis    = 5_000  // New-Order
	ceilingPaymentMillis     = 5_000  // Payment
	ceilingOrderStatusMillis = 5_000  // Order-Status
	ceilingDeliveryMillis    = 5_000  // Delivery
	ceilingStockLevelMillis  = 20_000 // Stock-Level

	// §5.2.2 transaction-mix minimums, as a percentage of all measured
	// transactions. The §5.2.3 dispatch weights target exactly these shares, so an
	// observed mix below a minimum means the run no longer exercises the workload
	// TPC-C prescribes.
	mixMinNewOrderPct    = 45.0 // New-Order
	mixMinPaymentPct     = 43.0 // Payment
	mixMinOrderStatusPct = 4.0  // Order-Status
	mixMinDeliveryPct    = 4.0  // Delivery
	mixMinStockLevelPct  = 4.0  // Stock-Level

	// mixTolerancePct is the absolute allowance below each mix minimum, so finite
	// samples don't fail on sampling noise. TPC-C validates the mix over a
	// multi-hour steady-state interval where that noise is negligible; a stress
	// CLI cannot guarantee one, so we widen the floor rather than flag every
	// short run non-compliant.
	mixTolerancePct = 1.0

	// minNewOrdersForValidity and minMeasurementsForValidity are project-chosen
	// floors for a statistically meaningful result. The formal TPC-C result
	// (§5.2.5) requires a sustained steady-state measurement interval that a
	// stress CLI cannot guarantee, so runs below these floors are flagged as
	// insufficient rather than asserting compliance.
	minNewOrdersForValidity    = 100
	minMeasurementsForValidity = 1_000
)

// Steady-state (3σ) constants for the paced-run spread check.
const (
	// steadySlotWidth is the wall-clock resolution of the New-Order completion
	// time series; steadySlotCount fixes its capacity (1 hour at 1s).
	steadySlotWidth = time.Second
	steadySlotCount = 3600

	// steadyBinCount is how many equal-width windows the measured interval is
	// re-bucketed into for the spread calculation.
	steadyBinCount = 30

	// minSteadySlots is the shortest interval (in slots) over which a spread can
	// be assessed; shorter runs are marked insufficient rather than judged.
	minSteadySlots = 30

	// minSteadyBins is the fewest non-empty windows needed for a meaningful spread.
	minSteadyBins = 4

	// steadyCVMax is the coefficient-of-variation ceiling for a steady run
	// (project heuristic: a burst-prone run exceeds this).
	steadyCVMax = 0.30
)

// txSpec couples one transaction type to its spec constants and metric suffix.
type txSpec struct {
	kind      txType
	name      string  // metric suffix, e.g. "new_order"
	ceilingMs float64 // §5.2.5 90th-percentile ceiling
	mixMinPct float64 // §5.2.2 mix minimum
}

// complianceTxTable is the canonical transaction set, in §5.2.3 mix order.
var complianceTxTable = []txSpec{
	{txNewOrder, "new_order", ceilingNewOrderMillis, mixMinNewOrderPct},
	{txPayment, "payment", ceilingPaymentMillis, mixMinPaymentPct},
	{txOrderStatus, "order_status", ceilingOrderStatusMillis, mixMinOrderStatusPct},
	{txDelivery, "delivery", ceilingDeliveryMillis, mixMinDeliveryPct},
	{txStockLevel, "stock_level", ceilingStockLevelMillis, mixMinStockLevelPct},
}

var (
	errObsCountMismatch = errors.New("tpcc compliance: unexpected observation count")
	errBucketMismatch   = errors.New("tpcc compliance: histogram bounds and bucket counts differ")
)

// txObservation is one transaction type's observed latency distribution over a
// run: the number of transactions plus the fixed-bucket histogram that bounds
// their response times. Keeping the histogram (rather than raw samples) bounds
// report memory on high-throughput runs. bucketCounts carries len(bounds)+1
// entries: the trailing count is the +Inf overflow bucket.
type txObservation struct {
	count        uint64
	bounds       []float64 // histogram bucket upper bounds, milliseconds, ascending
	bucketCounts []uint64  // observations per bucket; len(bounds)+1 (last is +Inf)
}

// quantile returns the q-th percentile of the histogram as an upper-bound bucket
// boundary, in milliseconds. The overflow (+Inf) bucket resolves to the last
// finite bound, matching the generic bench summary's histogram quantiles (an
// upper bound of the true percentile).
func (o txObservation) quantile(q float64) float64 {
	if o.count == 0 || len(o.bucketCounts) == 0 {
		return 0
	}

	target := uint64(float64(o.count-1)*q) + 1

	var cumulative uint64

	for i, bucketCount := range o.bucketCounts {
		cumulative += bucketCount
		if cumulative < target {
			continue
		}

		if i < len(o.bounds) {
			return o.bounds[i]
		}

		if len(o.bounds) > 0 {
			return o.bounds[len(o.bounds)-1]
		}
	}

	return 0
}

// Report is the machine-readable TPC-C compliance report. Verdict fields that do
// not apply to the run (unpaced) are nil.
type Report struct {
	Workload             string      `json:"workload"`
	Paced                bool        `json:"paced"`
	ComplianceApplicable bool        `json:"compliance_applicable"`
	ElapsedSeconds       float64     `json:"elapsed_seconds"`
	TpmC                 float64     `json:"tpm_c"`
	MixCompliant         *bool       `json:"mix_compliant"`
	ResponseCompliant    *bool       `json:"response_compliant"`
	MixTolerancePct      float64     `json:"mix_tolerance_pct"`
	Transactions         []TxReport  `json:"transactions"`
	Statistical          Statistical `json:"statistical"`
	Steadiness           Steadiness  `json:"steadiness"`
	Note                 string      `json:"note,omitempty"`
}

// TxReport is the per-transaction-type observation and, when applicable, its
// verdict against the §5.2.2 mix minimum (with the tolerance already applied) and
// the §5.2.5 response-time ceiling.
type TxReport struct {
	Name             string   `json:"name"`
	Count            uint64   `json:"count"`
	MixPercent       float64  `json:"mix_percent"`
	ThroughputPerSec float64  `json:"throughput_per_sec"`
	P50Ms            float64  `json:"p50_ms"`
	P90Ms            float64  `json:"p90_ms"`
	P95Ms            float64  `json:"p95_ms"`
	P99Ms            float64  `json:"p99_ms"`
	CeilingMs        *float64 `json:"ceiling_ms"`
	MixMinPercent    *float64 `json:"mix_min_percent"`
	MixPass          *bool    `json:"mix_pass"`
	ResponsePass     *bool    `json:"response_pass"`
}

// Statistical reports whether the run observed enough data to support a
// compliance claim, and why not when it is insufficient.
type Statistical struct {
	TotalMeasurements uint64 `json:"total_measurements"`
	NewOrders         uint64 `json:"new_orders"`
	Sufficient        bool   `json:"sufficient"`
	Reason            string `json:"reason,omitempty"`
}

// Steadiness is the 3σ steady-state check: the coefficient of variation (CV) of
// per-window New-Order throughput over the measurement window. Mean and Sigma
// are New-Orders-per-second; CV = Sigma/Mean.
type Steadiness struct {
	Status    string  `json:"status"` // "pass", "fail", "insufficient", "not_assessed"
	CV        float64 `json:"cv"`
	Mean      float64 `json:"mean_new_orders_per_sec"`
	Sigma     float64 `json:"sigma_new_orders_per_sec"`
	Buckets   int     `json:"buckets"`
	Truncated bool    `json:"truncated"`
	Reason    string  `json:"reason,omitempty"`
}

// reportOptions carries the run context a report is computed against. Deterministic:
// the caller supplies the elapsed measurement window rather than reading a clock.
type reportOptions struct {
	workload string
	paced    bool
	elapsed  time.Duration
	steady   steadySeries
}

// steady is a bounded New-Order completion time series: counts per fixed
// wall-clock slot, used by the report to assess steady-state spread without
// retaining raw samples. Safe for concurrent VUs.
type steady struct {
	start     time.Time
	width     time.Duration
	slots     []atomic.Uint64
	truncated atomic.Bool
}

func newSteady(start time.Time, width time.Duration, slots int) *steady {
	return &steady{start: start, width: width, slots: make([]atomic.Uint64, slots)}
}

// record buckets one New-Order completion into the slot covering now. Slots past
// capacity are not recorded and the series is marked truncated.
func (s *steady) record(now time.Time) {
	elapsed := now.Sub(s.start)
	if elapsed < 0 {
		elapsed = 0
	}

	idx := int64(elapsed / s.width)
	if idx >= int64(len(s.slots)) {
		s.truncated.Store(true)

		return
	}

	s.slots[idx].Add(1)
}

// steadySeries is the read-only New-Order completion counts per slot as passed to
// the report.
type steadySeries struct {
	counts    []uint64 // per-slot counts over the measured window
	truncated bool
}

// snapshot copies the per-slot counts that fall within elapsed, up to capacity.
func (s *steady) snapshot(elapsed time.Duration) steadySeries {
	active := int(elapsed / s.width)
	if active < 0 {
		active = 0
	}

	if active > len(s.slots) {
		active = len(s.slots)
	}

	counts := make([]uint64, active)
	for i := range active {
		counts[i] = s.slots[i].Load()
	}

	return steadySeries{counts: counts, truncated: s.truncated.Load()}
}

// complianceReport aggregates the per-transaction observations into a Report.
// Verdicts (mix and response-time ceilings) are computed only for paced runs;
// unpaced runs get raw statistics with compliance marked not applicable.
func complianceReport(obs []txObservation, opts reportOptions) (Report, error) {
	if len(obs) != len(complianceTxTable) {
		return Report{}, fmt.Errorf(
			"%w: got %d observations, want %d",
			errObsCountMismatch, len(obs), len(complianceTxTable),
		)
	}

	var (
		total         uint64
		newOrderCount uint64
	)

	for i, o := range obs {
		// A histogram with observations must carry the +Inf overflow bucket; an
		// empty observation (no transactions recorded) is valid as-is.
		if o.count > 0 && len(o.bucketCounts) != len(o.bounds)+1 {
			return Report{}, fmt.Errorf(
				"%w: %s has %d bounds and %d bucket counts",
				errBucketMismatch, complianceTxTable[i].name, len(o.bounds), len(o.bucketCounts),
			)
		}

		total += o.count
		if complianceTxTable[i].kind == txNewOrder {
			newOrderCount = o.count
		}
	}

	elapsedSeconds := opts.elapsed.Seconds()

	r := Report{
		Workload:             opts.workload,
		Paced:                opts.paced,
		ComplianceApplicable: opts.paced,
		ElapsedSeconds:       elapsedSeconds,
		MixTolerancePct:      mixTolerancePct,
		Transactions:         make([]TxReport, 0, len(obs)),
		Statistical: Statistical{
			TotalMeasurements: total,
			NewOrders:         newOrderCount,
		},
	}

	if elapsedSeconds > 0 {
		r.TpmC = float64(newOrderCount) / (elapsedSeconds / 60)
	}

	mixPass := true
	respPass := true

	for i, o := range obs {
		spec := complianceTxTable[i]

		txr := TxReport{
			Name:             spec.name,
			Count:            o.count,
			ThroughputPerSec: throughput(o.count, elapsedSeconds),
			P50Ms:            o.quantile(0.50),
			P90Ms:            o.quantile(0.90),
			P95Ms:            o.quantile(0.95),
			P99Ms:            o.quantile(0.99),
		}

		if total > 0 {
			txr.MixPercent = float64(o.count) / float64(total) * 100
		}

		if opts.paced {
			ceiling := spec.ceilingMs
			// Surface the effective floor (spec minimum minus tolerance) so a
			// machine consumer can reconcile the verdict with mix_tolerance_pct.
			floorPct := spec.mixMinPct - mixTolerancePct

			mixOK := txr.MixPercent >= floorPct

			txr.CeilingMs = &ceiling
			txr.MixMinPercent = &floorPct
			txr.MixPass = &mixOK

			// A transaction type never measured (count 0) gets no response verdict:
			// its mix already fails below the floor, and we must not claim a pass.
			if o.count > 0 {
				respOK := txr.P90Ms <= spec.ceilingMs

				txr.ResponsePass = &respOK
				respPass = respPass && respOK
			}

			mixPass = mixPass && mixOK
		}

		r.Transactions = append(r.Transactions, txr)
	}

	if !opts.paced {
		const unpacedNote = "unpaced stress run: §5.2.5 compliance " +
			"(mix, response-time ceilings, statistical validity) is not applicable"

		r.Statistical.Reason = unpacedNote
		r.Steadiness = Steadiness{Status: "not_assessed", Reason: unpacedNote}
		r.Note = unpacedNote

		return r, nil
	}

	r.MixCompliant = &mixPass
	r.ResponseCompliant = &respPass
	r.Steadiness = computeSteadiness(opts.steady)

	r.Statistical.Sufficient, r.Statistical.Reason = statisticalSufficiency(newOrderCount, total)

	return r, nil
}

// statisticalSufficiency reports whether the sample is large enough to support a
// compliance claim and, if not, why.
func statisticalSufficiency(newOrderCount, total uint64) (sufficient bool, reason string) {
	switch {
	case newOrderCount < minNewOrdersForValidity:
		return false, fmt.Sprintf(
			"too few New-Order transactions (%d < %d) to establish a statistically valid tpmC",
			newOrderCount, minNewOrdersForValidity,
		)
	case total < minMeasurementsForValidity:
		return false, fmt.Sprintf(
			"too few measurements (%d < %d) for stable percentiles and mix",
			total, minMeasurementsForValidity,
		)
	default:
		return true, ""
	}
}

// computeSteadiness derives the 3σ steady-state indicator from the per-slot
// New-Order completion series.
func computeSteadiness(series steadySeries) Steadiness {
	active := len(series.counts)

	if active == 0 {
		return Steadiness{Status: "insufficient", Reason: "no New-Order completions recorded"}
	}

	if active < minSteadySlots {
		return Steadiness{
			Status:    "insufficient",
			Truncated: series.truncated,
			Reason: fmt.Sprintf(
				"measurement window (%ds) too short for a steady-state check (need >= %ds)",
				active, minSteadySlots,
			),
		}
	}

	rates := rebucketRates(series.counts, steadyBinCount)

	var nonEmpty int

	for _, rate := range rates {
		if rate > 0 {
			nonEmpty++
		}
	}

	if nonEmpty < minSteadyBins {
		return Steadiness{
			Status:    "insufficient",
			Truncated: series.truncated,
			Reason:    fmt.Sprintf("too few active steady-state windows (%d) to assess spread", nonEmpty),
		}
	}

	mean, sigma := meanStd(rates)

	cv := 0.0
	if mean > 0 {
		cv = sigma / mean
	}

	status := "pass"
	if cv > steadyCVMax {
		status = "fail"
	}

	reason := ""
	if series.truncated {
		reason = "steady-state window truncated; spread computed over the recorded prefix"
	}

	return Steadiness{
		Status:    status,
		CV:        cv,
		Mean:      mean,
		Sigma:     sigma,
		Buckets:   len(rates),
		Truncated: series.truncated,
		Reason:    reason,
	}
}

// rebucketRates folds per-slot New-Order counts into up to binCount equal-width
// windows and returns each window's New-Orders-per-second rate, so a partial
// trailing window stays comparable to full-width ones.
func rebucketRates(counts []uint64, binCount int) []float64 {
	n := len(counts)
	if n == 0 {
		return nil
	}

	binCount = min(binCount, n)

	slotsPerBin := ceilDiv(n, binCount)
	rates := make([]float64, 0, binCount)

	for start := 0; start < n; start += slotsPerBin {
		end := min(start+slotsPerBin, n)

		var total uint64
		for _, c := range counts[start:end] {
			total += c
		}

		rates = append(rates, float64(total)/float64(end-start))
	}

	return rates
}

// meanStd returns the mean and population standard deviation of values, via the
// single-pass E[x^2]-mean^2 form (exact for the small integer counts used here).
func meanStd(values []float64) (mean, sigma float64) {
	if len(values) == 0 {
		return 0, 0
	}

	var (
		sum   float64
		sumSq float64
	)

	for _, v := range values {
		sum += v
		sumSq += v * v
	}

	n := float64(len(values))
	mean = sum / n

	variance := sumSq/n - mean*mean
	if variance < 0 {
		variance = 0
	}

	return mean, math.Sqrt(variance)
}

func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}

// throughput is count per second over the measurement window, or 0 when the
// window or count is empty.
func throughput(count uint64, elapsedSeconds float64) float64 {
	if count == 0 || elapsedSeconds <= 0 {
		return 0
	}

	return float64(count) / elapsedSeconds
}

// text renders the human-readable compliance block written to stderr.
func (r *Report) text() string {
	var b strings.Builder

	fmt.Fprintf(&b, "=== TPC-C compliance report (%s) ===\n", r.Workload)
	fmt.Fprintf(&b, "elapsed=%.1fs paced=%t\n", r.ElapsedSeconds, r.Paced)

	if !r.ComplianceApplicable {
		fmt.Fprintln(&b, "compliance: NOT APPLICABLE (unpaced stress run; raw stats below)")
	} else {
		fmt.Fprintf(&b, "mix: %s, response-time: %s\n",
			passFail(r.MixCompliant), passFail(r.ResponseCompliant))
	}

	fmt.Fprintf(&b, "%-12s %8s %7s %9s %8s %8s %8s %8s",
		"transaction", "count", "mix%", "tps", "p50", "p90", "p95", "p99")

	if r.ComplianceApplicable {
		fmt.Fprintf(&b, " %8s %8s", "ceiling", "status")
	}

	fmt.Fprintln(&b)

	for _, t := range r.Transactions {
		fmt.Fprintf(&b, "%-12s %8d %6.1f%% %9.3f %7.0fms %7.0fms %7.0fms %7.0fms",
			t.Name, t.Count, t.MixPercent, t.ThroughputPerSec,
			t.P50Ms, t.P90Ms, t.P95Ms, t.P99Ms)

		if r.ComplianceApplicable {
			fmt.Fprintf(&b, " %7.0fs %8s", *t.CeilingMs/1000, passFail(t.ResponsePass))
		}

		fmt.Fprintln(&b)
	}

	if !r.ComplianceApplicable {
		return b.String()
	}

	fmt.Fprintf(&b, "tpmC: %.2f\n", r.TpmC)

	if r.Statistical.Sufficient {
		fmt.Fprintf(&b, "statistical validity: SUFFICIENT (%d New-Orders, %d measurements)\n",
			r.Statistical.NewOrders, r.Statistical.TotalMeasurements)
	} else {
		fmt.Fprintf(&b, "statistical validity: INSUFFICIENT — %s\n", r.Statistical.Reason)
	}

	fmt.Fprintf(&b, "steadiness (3σ): %s", strings.ToUpper(r.Steadiness.Status))

	if r.Steadiness.Status == "pass" || r.Steadiness.Status == "fail" {
		fmt.Fprintf(&b, " — CV=%.3f σ=%.3f/s μ=%.3f/s over %d windows",
			r.Steadiness.CV, r.Steadiness.Sigma, r.Steadiness.Mean, r.Steadiness.Buckets)
	} else if r.Steadiness.Reason != "" {
		fmt.Fprintf(&b, " — %s", r.Steadiness.Reason)
	}

	fmt.Fprintln(&b)

	if r.Steadiness.Truncated && r.Steadiness.Reason != "" {
		fmt.Fprintf(&b, "  note: %s\n", r.Steadiness.Reason)
	}

	return b.String()
}

// passFail renders a verdict pointer as PASS, FAIL, or - when absent.
func passFail(v *bool) string {
	switch {
	case v == nil:
		return "-"
	case *v:
		return "PASS"
	default:
		return "FAIL"
	}
}

// emitComplianceReport collects the run's TPC-C metrics, computes the compliance
// report, and writes the human block to stderr plus one JSON line to stdout.
// Emitting is report-only: a non-compliant or unavailable report never changes the
// process exit status.
func (w *workload) emitComplianceReport(b *bench.Bench) {
	metrics, err := b.CollectedMetrics()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpcc: compliance report skipped: %v\n", err)

		return
	}

	obs := make([]txObservation, len(complianceTxTable))

	for i, spec := range complianceTxTable {
		dur, ok := metrics["tpcc_"+spec.name+"_duration"]
		if !ok {
			continue // no transactions of this type recorded
		}

		obs[i] = txObservation{
			count:        dur.Count,
			bounds:       dur.Bounds,
			bucketCounts: dur.Buckets,
		}
	}

	elapsed := time.Since(w.measureStart)

	var series steadySeries
	if w.steady != nil {
		series = w.steady.snapshot(elapsed)
	}

	// Compliance verdicts apply only when pacing is on (TPC-C keying and think
	// times, §5.2.5): that slow, spec-style run is the only one whose observed mix
	// and response times can be judged against the specification. An unpaced run is
	// a raw stress test with no steady-state guarantee.
	report, err := complianceReport(obs, reportOptions{
		workload: w.Name(),
		paced:    w.pacing,
		elapsed:  elapsed,
		steady:   series,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpcc: compliance report unavailable: %v\n", err)

		return
	}

	fmt.Fprint(os.Stderr, report.text())

	if err := json.NewEncoder(os.Stdout).Encode(map[string]Report{"compliance": report}); err != nil {
		fmt.Fprintf(os.Stderr, "tpcc: compliance JSON output failed: %v\n", err)
	}
}
