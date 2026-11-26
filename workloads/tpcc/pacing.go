package tpcc

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

const ceilingTolerance = 0.05

var pacingBounds = func() []float64 {
	bounds := make([]float64, 121)
	for i := range bounds {
		bounds[i] = float64(i)
	}

	return bounds
}()

type WaitSummary struct {
	Count            uint64  `json:"count"`
	SumSeconds       float64 `json:"sum_seconds"`
	MinSeconds       float64 `json:"min_seconds"`
	AverageSeconds   float64 `json:"average_seconds"`
	MaxSeconds       float64 `json:"max_seconds"`
	Truncated        uint64  `json:"truncated"`
	TruncatedSeconds float64 `json:"truncated_seconds"`
}

func (s *WaitSummary) add(seconds float64, complete bool) {
	if !complete {
		s.Truncated++
		s.TruncatedSeconds += seconds

		return
	}

	if s.Count == 0 {
		s.MinSeconds = seconds
	}

	s.Count++
	s.SumSeconds += seconds
	s.MinSeconds = min(s.MinSeconds, seconds)
	s.MaxSeconds = max(s.MaxSeconds, seconds)
	s.AverageSeconds = s.SumSeconds / float64(s.Count)
}

type PacingTransaction struct {
	Name                   string      `json:"name"`
	Mix                    float64     `json:"assumed_mix"`
	KeyingSeconds          float64     `json:"assumed_keying_seconds"`
	ThinkMeanSeconds       float64     `json:"assumed_think_mean_seconds"`
	Keying                 WaitSummary `json:"keying"`
	Think                  WaitSummary `json:"think"`
	Responses              uint64      `json:"responses"`
	ResponseSumSeconds     float64     `json:"response_sum_seconds"`
	ResponseAverageSeconds *float64    `json:"response_average_seconds"`
	ThinkBounds            []float64   `json:"think_bounds_seconds,omitempty"`
	ThinkBuckets           []uint64    `json:"think_buckets,omitempty"`
}

type WarehousePacing struct {
	Warehouse       int64   `json:"warehouse"`
	Terminals       int     `json:"terminals"`
	NewOrders       uint64  `json:"new_orders"`
	TpmC            float64 `json:"tpm_c"`
	CeilingFraction float64 `json:"ceiling_fraction"`
	Verdict         string  `json:"verdict"`
}

type PacingReport struct {
	Model                   string              `json:"model"`
	Applicable              bool                `json:"compliance_applicable"`
	FixedBinding            bool                `json:"fixed_home_warehouse_binding"`
	Simulated               bool                `json:"simulated"`
	Warehouses              int64               `json:"warehouses"`
	MeasurementSeconds      float64             `json:"measurement_seconds"`
	ExpectedMaxTpmC         float64             `json:"expected_max_tpmc"`
	ModelMaxTpmC            float64             `json:"model_max_tpmc"`
	ResponseAdjustedMaxTpmC *float64            `json:"response_adjusted_max_tpmc"`
	MinimumTpmC             float64             `json:"minimum_tpmc"`
	MinimumPerWarehouse     float64             `json:"minimum_tpmc_per_warehouse"`
	MeasuredTpmC            float64             `json:"measured_tpmc"`
	CeilingFraction         float64             `json:"ceiling_fraction"`
	Tolerance               float64             `json:"ceiling_tolerance"`
	Verdict                 string              `json:"verdict"`
	Transactions            []PacingTransaction `json:"transactions"`
	WarehouseRates          []WarehousePacing   `json:"warehouse_rates"`
	DeliverySupported       bool                `json:"deferred_delivery_supported"`
	DeliveryCount           uint64              `json:"delivery_count"`
	DeliveryLate            uint64              `json:"delivery_over_80_seconds"`
	DeliveryFailed          uint64              `json:"delivery_failed"`
	DeliveryPending         uint64              `json:"delivery_pending"`
	DeliveryBounds          []float64           `json:"delivery_bounds_seconds,omitempty"`
	DeliveryBuckets         []uint64            `json:"delivery_buckets,omitempty"`
	Note                    string              `json:"note"`
}

type pacing struct {
	mu                                                   sync.Mutex
	start, end                                           time.Time
	transactions                                         [5]PacingTransaction
	warehouses                                           []WarehousePacing
	wait                                                 [5][2]*bench.HistogramHandle
	delivery                                             *bench.HistogramHandle
	deliveryLate                                         *bench.CounterHandle
	deliveryCount, deliveryFailed, deliveryPending, late uint64
	deliveryBuckets                                      []uint64
}

func (w *workload) initPacing(d *bench.MetricDeclarations, workers int) {
	if !w.pacing {
		return
	}

	p := &pacing{warehouses: make([]WarehousePacing, w.warehouses), deliveryBuckets: make([]uint64, len(pacingBounds)+1)}
	for i := range p.warehouses {
		terminals := workers / int(w.warehouses)
		if i < workers%int(w.warehouses) {
			terminals++
		}

		if w.strict != nil {
			terminals = 10
		}

		if w.spreadWarehouses {
			terminals = 0
		}

		p.warehouses[i] = WarehousePacing{Warehouse: w.warehouseStart + int64(i), Terminals: terminals}
	}

	for i, name := range txNames {
		mix := float64(txWeights[i]) / 100
		if w.strict != nil {
			mix = float64([]int{10, 10, 1, 1, 1}[i]) / 23
		}

		p.transactions[i] = PacingTransaction{
			Name: name, Mix: mix, KeyingSeconds: float64(keyingTime[name]),
			ThinkMeanSeconds: thinkTimeMean[name],
		}
		if i == 0 {
			p.transactions[i].ThinkBounds = pacingBounds
			p.transactions[i].ThinkBuckets = make([]uint64, len(pacingBounds)+1)
		}

		for kind, wait := range []string{"keying", "think"} {
			p.wait[i][kind] = d.Histogram("tpcc_"+name+"_"+wait+"_seconds", bench.Unit("s"), bench.Bounds(pacingBounds...),
				bench.LabelValues("phase", "warmup", "measurement", "boundary", "drain"),
				bench.LabelValues("wait_outcome", "full", "truncated"))
		}
	}

	if w.strict != nil {
		p.delivery = d.Histogram("tpcc_delivery_queue_seconds", bench.Unit("s"), bench.Bounds(pacingBounds...),
			bench.LabelValues("phase", "warmup", "measurement", "boundary", "drain"),
			bench.LabelValues("outcome", "success", "error", "pending"))
		p.deliveryLate = d.Counter("tpcc_delivery_over_80_seconds")
	}

	w.pace = p
}

func (p *pacing) window(start, end time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.start, p.end = start, end
}

func (p *pacing) phase(start, end time.Time) string {
	if p.start.IsZero() || end.Before(p.start) {
		return "warmup"
	}

	if start.Before(p.start) {
		return "boundary"
	}

	if !p.end.IsZero() && !end.Before(p.end) {
		return "drain"
	}

	return "measurement"
}

func addBucket(buckets []uint64, seconds float64) {
	for i, bound := range pacingBounds {
		if seconds <= bound {
			buckets[i]++

			return
		}
	}

	buckets[len(buckets)-1]++
}

func (p *pacing) appliedWait(ctx context.Context, idx, kind int, start, end time.Time, complete bool) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	seconds := max(0, end.Sub(start).Seconds())
	phase := p.phase(start, end)

	outcome := "full"
	if !complete {
		outcome = "truncated"
	}

	p.wait[idx][kind].Record(ctx, seconds, bench.LabelValue("phase", phase), bench.LabelValue("wait_outcome", outcome))

	truncatedMeasured := !complete && !p.start.IsZero() && !start.Before(p.start) &&
		(p.end.IsZero() || start.Before(p.end))
	if phase != "measurement" && !truncatedMeasured {
		return
	}

	summary := &p.transactions[idx].Keying
	if kind == 1 {
		summary = &p.transactions[idx].Think
	}

	summary.add(seconds, complete)

	if idx == 0 && kind == 1 && complete {
		addBucket(p.transactions[0].ThinkBuckets, seconds)
	}
}

func (w *workload) pacedWait(ctx context.Context, name string, kind int, seconds float64) error {
	start := time.Now()
	err := sleepSeconds(ctx, seconds)
	w.pace.appliedWait(ctx, transactionIndex(name), kind, start, time.Now(), err == nil)

	return err
}

func transactionIndex(name string) int {
	for i, tx := range txNames {
		if tx == name {
			return i
		}
	}

	return 0
}

func (p *pacing) response(idx int, warehouse int64, started, completed time.Time, err error) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.phase(started, completed) != "measurement" {
		return
	}

	tx := &p.transactions[idx]
	tx.Responses++
	tx.ResponseSumSeconds += completed.Sub(started).Seconds()

	if idx == 0 && err == nil && len(p.warehouses) > 0 {
		i := warehouse - p.warehouses[0].Warehouse
		if i >= 0 && i < int64(len(p.warehouses)) {
			p.warehouses[i].NewOrders++
		}
	}
}

func (p *pacing) delivered(ctx context.Context, result *deliveryResult, err error, pending bool) {
	if p == nil || p.delivery == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	seconds := max(0, result.Completed.Sub(result.Queued).Seconds())
	phase := p.phase(result.Queued, result.Completed)

	outcome := "success"
	if err != nil {
		outcome = "error"
	}

	if pending {
		outcome = "pending"
	}

	p.delivery.Record(ctx, seconds, bench.LabelValue("phase", phase), bench.LabelValue("outcome", outcome))

	if p.start.IsZero() || result.Queued.Before(p.start) || (!p.end.IsZero() && !result.Queued.Before(p.end)) {
		return
	}

	p.deliveryCount++
	if pending {
		p.deliveryPending++
	} else if err != nil {
		p.deliveryFailed++
	}

	if seconds > 80 {
		p.late++
		p.deliveryLate.Add(ctx, 1)
	}

	addBucket(p.deliveryBuckets, seconds)
}

func terminalCeiling(weights, response []float64) float64 {
	budget := 0.0

	for i, name := range txNames {
		seconds := float64(keyingTime[name]) + thinkTimeMean[name]
		if response != nil {
			seconds += response[i]
		}

		budget += weights[i] * seconds
	}

	return 600 * weights[0] / budget
}

func ceilingVerdict(measured, ceiling, minimum float64, applicable bool) string {
	if !applicable {
		return "not_applicable"
	}

	if measured > ceiling*(1+ceilingTolerance) {
		return "exceeds_terminal_model_ceiling"
	}

	if measured < minimum {
		return "under_loaded"
	}

	return "pass"
}

func (w *workload) pacingReport(elapsed time.Duration) *PacingReport {
	p := w.pace
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	weights := make([]float64, 5)
	response := make([]float64, 5)
	complete := true

	for i := range p.transactions {
		weights[i] = p.transactions[i].Mix

		tx := &p.transactions[i]
		if tx.Responses == 0 {
			complete = false
		} else {
			response[i] = tx.ResponseSumSeconds / float64(tx.Responses)
			tx.ResponseAverageSeconds = &response[i]
		}
	}

	specWeights := make([]float64, len(txWeights))
	for i, weight := range txWeights {
		specWeights[i] = float64(weight) / 100
	}

	spec := math.Round(terminalCeiling(specWeights, nil)*100) / 100
	model := terminalCeiling(weights, nil)
	applicable := !w.spreadWarehouses && !w.isPicodata && (w.strict != nil || !w.terminalPopulationMismatch)

	r := &PacingReport{
		Model: "stress", Applicable: applicable, FixedBinding: !w.spreadWarehouses,
		Simulated: w.driverType == bench.DriverNoop, Warehouses: w.warehouses, MeasurementSeconds: elapsed.Seconds(),
		ExpectedMaxTpmC: spec * float64(w.warehouses), ModelMaxTpmC: model * float64(w.warehouses),
		MinimumTpmC: 9 * float64(w.warehouses), MinimumPerWarehouse: 9, Tolerance: ceilingTolerance,
		Transactions:      append([]PacingTransaction(nil), p.transactions[:]...),
		WarehouseRates:    append([]WarehousePacing(nil), p.warehouses...),
		DeliverySupported: w.strict != nil, DeliveryCount: p.deliveryCount, DeliveryFailed: p.deliveryFailed,
		DeliveryPending: p.deliveryPending, DeliveryLate: p.late,
		Note: "Diagnostic only: 5% ceiling tolerance allows finite-sample variation; " +
			"short runs and incomplete waits do not establish steady state or audited TPC-C compliance.",
	}
	if w.strict != nil {
		r.Model = "strict"
	}

	if complete {
		adjusted := terminalCeiling(weights, response) * float64(w.warehouses)
		r.ResponseAdjustedMaxTpmC = &adjusted
	}

	if r.DeliverySupported {
		r.DeliveryBounds = pacingBounds
		r.DeliveryBuckets = p.deliveryBuckets
	}

	var orders uint64

	for i := range r.WarehouseRates {
		rate := &r.WarehouseRates[i]
		if elapsed > 0 {
			rate.TpmC = float64(rate.NewOrders) * 60 / elapsed.Seconds()
		}

		rate.CeilingFraction = rate.TpmC / spec
		rate.Verdict = ceilingVerdict(rate.TpmC, spec, 9, applicable && rate.Terminals == 10)
		orders += rate.NewOrders
	}

	if elapsed > 0 {
		r.MeasuredTpmC = float64(orders) * 60 / elapsed.Seconds()
	}

	if r.ExpectedMaxTpmC > 0 {
		r.CeilingFraction = r.MeasuredTpmC / r.ExpectedMaxTpmC
	}

	r.Verdict = ceilingVerdict(r.MeasuredTpmC, r.ExpectedMaxTpmC, r.MinimumTpmC, applicable)

	return r
}

func (r *PacingReport) text() string {
	var out strings.Builder

	status := "PASS"
	if r.Verdict == "not_applicable" {
		status = "NOT APPLICABLE"
	} else if r.Verdict != "pass" {
		status = "FAIL"
	}

	fmt.Fprintf(&out, "Terminal ceiling: %s — %s (diagnostic, %s)\n", status, r.Verdict, r.Model)
	fmt.Fprintf(&out, "  measured %.3f tpmC; maximum %.3f; selected-model %.3f; minimum %.3f; fraction %.3f\n",
		r.MeasuredTpmC, r.ExpectedMaxTpmC, r.ModelMaxTpmC, r.MinimumTpmC, r.CeilingFraction)

	if r.ResponseAdjustedMaxTpmC != nil {
		fmt.Fprintf(&out, "  response-adjusted maximum %.3f tpmC; tolerance %.1f%%; fixed binding=%t\n",
			*r.ResponseAdjustedMaxTpmC, 100*r.Tolerance, r.FixedBinding)
	}

	for i := range r.Transactions {
		tx := &r.Transactions[i]
		fmt.Fprintf(&out, "  %s assumptions: mix=%.5f keying=%.0fs mean think=%.0fs\n",
			tx.Name, tx.Mix, tx.KeyingSeconds, tx.ThinkMeanSeconds)
		fmt.Fprintf(&out, "  %s keying min/avg/max %.6f/%.6f/%.6fs, think %.6f/%.6f/%.6fs; truncated %d/%d\n",
			tx.Name, tx.Keying.MinSeconds, tx.Keying.AverageSeconds, tx.Keying.MaxSeconds,
			tx.Think.MinSeconds, tx.Think.AverageSeconds, tx.Think.MaxSeconds, tx.Keying.Truncated, tx.Think.Truncated)
	}

	for _, wh := range r.WarehouseRates {
		fmt.Fprintf(&out, "  warehouse %s: terminals=%d, %.3f tpmC, %s\n",
			strconv.FormatInt(wh.Warehouse, 10), wh.Terminals, wh.TpmC, wh.Verdict)
	}

	if r.DeliverySupported {
		fmt.Fprintf(&out, "  deferred Delivery: count=%d over80s=%d failed=%d pending=%d\n",
			r.DeliveryCount, r.DeliveryLate, r.DeliveryFailed, r.DeliveryPending)
	}

	fmt.Fprintln(&out, r.Note)

	return out.String()
}
