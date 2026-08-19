package tpcc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
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
// report memory on high-throughput runs.
type txObservation struct {
	count        uint64
	bounds       []float64 // histogram bucket upper bounds, milliseconds, ascending
	bucketCounts []uint64  // observations per bucket; aligned with bounds
}

// quantile returns the q-th percentile of the histogram as an upper-bound bucket
// boundary, in milliseconds. The estimate matches the generic bench summary's
// histogram quantiles (an upper bound of the true percentile).
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
	Transactions         []TxReport  `json:"transactions"`
	Statistical          Statistical `json:"statistical"`
	Note                 string      `json:"note,omitempty"`
}

// TxReport is the per-transaction-type observation and, when applicable, its
// verdict against the §5.2.2 mix minimum and the §5.2.5 response-time ceiling.
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

// reportOptions carries the run context a report is computed against. Deterministic:
// the caller supplies the elapsed measurement window rather than reading a clock.
type reportOptions struct {
	workload string
	paced    bool
	elapsed  time.Duration
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
		if len(o.bounds) != len(o.bucketCounts) {
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
			minPct := spec.mixMinPct

			mixOK := txr.MixPercent >= spec.mixMinPct-mixTolerancePct
			respOK := txr.P90Ms <= spec.ceilingMs

			txr.CeilingMs = &ceiling
			txr.MixMinPercent = &minPct
			txr.MixPass = &mixOK
			txr.ResponsePass = &respOK

			mixPass = mixPass && mixOK
			respPass = respPass && respOK
		}

		r.Transactions = append(r.Transactions, txr)
	}

	if !opts.paced {
		const unpacedNote = "unpaced stress run: §5.2.5 compliance " +
			"(mix, response-time ceilings, statistical validity) is not applicable"

		r.Statistical.Reason = unpacedNote
		r.Note = unpacedNote

		return r, nil
	}

	r.MixCompliant = &mixPass
	r.ResponseCompliant = &respPass
	r.Statistical.Sufficient = true

	switch {
	case newOrderCount < minNewOrdersForValidity:
		r.Statistical.Sufficient = false
		r.Statistical.Reason = fmt.Sprintf(
			"too few New-Order transactions (%d < %d) to establish a statistically valid tpmC",
			newOrderCount, minNewOrdersForValidity,
		)
	case total < minMeasurementsForValidity:
		r.Statistical.Sufficient = false
		r.Statistical.Reason = fmt.Sprintf(
			"too few measurements (%d < %d) for stable percentiles and mix",
			total, minMeasurementsForValidity,
		)
	}

	return r, nil
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

	// Compliance verdicts apply only when pacing is on (TPC-C keying and think
	// times, §5.2.5): that slow, spec-style run is the only one whose observed mix
	// and response times can be judged against the specification. An unpaced run is
	// a raw stress test with no steady-state guarantee.
	report, err := complianceReport(obs, reportOptions{
		workload: w.Name(),
		paced:    w.pacing,
		elapsed:  time.Since(w.measureStart),
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
