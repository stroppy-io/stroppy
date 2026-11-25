package tpcc

import (
	"fmt"
	"math"
	"time"
)

type strictObservation struct {
	Count       uint64  `json:"count"`
	Failed      uint64  `json:"failed"`
	Drain       uint64  `json:"drain"`
	Warmup      uint64  `json:"warmup"`
	Crossing    uint64  `json:"crossing"`
	SumSeconds  float64 `json:"sum_seconds"`
	MaxSeconds  float64 `json:"max_seconds"`
	WithinLimit uint64  `json:"within_limit"`
	P90Seconds  float64 `json:"p90_seconds"`
	buckets     []uint64
}

type strictStatistics struct {
	Transactions            [5]strictObservation `json:"transactions"`
	DeliveryCompleted       uint64               `json:"delivery_completed"`
	DeliveryFailed          uint64               `json:"delivery_failed"`
	DeliveryWithin80Seconds uint64               `json:"delivery_within_80_seconds"`
	DeliverySkipped         uint64               `json:"delivery_skipped"`
	DeliveryPending         uint64               `json:"delivery_pending"`
}

type TerminalReport struct {
	Model              string           `json:"model"`
	Terminals          int              `json:"terminals"`
	ExecutionWorkers   int              `json:"execution_workers"`
	MeasurementSeconds float64          `json:"measurement_seconds"`
	WarmupSeconds      float64          `json:"warmup_seconds"`
	DeliveryResults    string           `json:"delivery_results"`
	Statistics         strictStatistics `json:"statistics"`
	MixPass            bool             `json:"mix_pass"`
	ResponsePass       bool             `json:"response_pass"`
	DeliveryPass       bool             `json:"delivery_pass"`
	MeasurementPass    bool             `json:"measurement_pass"`
	Audited            bool             `json:"audited"`
	Note               string           `json:"note"`
}

func (s *strictStatistics) record(idx int, started, completed, measurement, end time.Time, err error) {
	o := &s.Transactions[idx]
	if completed.Before(measurement) {
		o.Warmup++

		return
	}

	if started.Before(measurement) {
		o.Crossing++

		return
	}

	if !completed.Before(end) {
		o.Drain++

		return
	}

	o.Count++
	if err != nil {
		o.Failed++
	}

	seconds := completed.Sub(started).Seconds()
	o.SumSeconds += seconds
	o.MaxSeconds = max(o.MaxSeconds, seconds)

	limit := 5.0
	if idx == int(txStockLevel) {
		limit = 20
	}

	if seconds < limit {
		o.WithinLimit++
	}
	// Millisecond bins preserve the 100 ms allowance in §5.2.5.6 without raw samples.
	if o.buckets == nil {
		o.buckets = make([]uint64, 100001)
	}

	bucket := min(int(math.Ceil(seconds*1000)), len(o.buckets)-1)
	o.buckets[max(0, bucket)]++
}

func (o *strictObservation) p90() float64 {
	target := (o.Count*9 + 9) / 10

	var sum uint64
	for i, count := range o.buckets {
		sum += count
		if sum >= target {
			if i == len(o.buckets)-1 {
				return o.MaxSeconds
			}

			return float64(i) / 1000
		}
	}

	return 0
}

func (s *strictStatistics) delivery(result *deliveryResult, measurement, end time.Time, err error) {
	if result.Queued.Before(measurement) || !result.Queued.Before(end) {
		return
	}

	if err != nil {
		s.DeliveryFailed++

		return
	}

	s.DeliveryCompleted++
	if result.Completed.Sub(result.Queued) < 80*time.Second {
		s.DeliveryWithin80Seconds++
	}

	if len(result.SkippedDistricts) > 0 {
		s.DeliverySkipped++
	}
}

func (m *terminalModel) report() TerminalReport {
	m.mu.Lock()
	defer m.mu.Unlock()

	r := TerminalReport{
		Model: "strict", Terminals: m.terminals, ExecutionWorkers: m.workers,
		MeasurementSeconds: m.duration.Seconds(), WarmupSeconds: m.warmup.Seconds(), DeliveryResults: m.resultsPath,
		Statistics: m.stats, MixPass: true, ResponsePass: true,
		Note: "Unaudited terminal-model diagnostics, not a TPC-C score. " +
			"Steady state, full terminal I/O, data generation, ACID, checkpoint, eight-hour capacity, " +
			"pricing, disclosure, and independent audit require separate verification.",
	}

	var total uint64
	for _, o := range m.stats.Transactions {
		total += o.Count
	}

	for i, o := range m.stats.Transactions {
		r.Statistics.Transactions[i].P90Seconds = o.p90()
		if o.Failed > 0 || (i > 0 && float64(o.Count)*100 < float64(total)*complianceTxTable[i].mixMinPct) {
			r.MixPass = false
		}

		if o.Count == 0 || o.Failed > 0 || o.WithinLimit*10 < o.Count*9 ||
			o.p90()+.1 < o.SumSeconds/float64(max(1, o.Count)) {
			r.ResponsePass = false
		}
	}

	if total == 0 {
		r.MixPass = false
	}

	delivery := m.stats.DeliveryCompleted
	r.DeliveryPass = delivery > 0 && m.stats.DeliveryFailed == 0 && m.stats.DeliveryPending == 0 &&
		m.stats.DeliveryWithin80Seconds*10 >= delivery*9 &&
		m.stats.DeliverySkipped <= max(uint64(1), delivery/100)

	elapsed := m.cutoff.Sub(m.measurement)
	if m.started.IsZero() {
		elapsed = 0
	}

	r.MeasurementSeconds = min(max(0, elapsed.Seconds()), m.duration.Seconds())
	r.MeasurementPass = r.MeasurementSeconds >= 2*time.Hour.Seconds()

	return r
}

func (r *TerminalReport) text() string {
	return fmt.Sprintf("TPC-C strict terminal model (unaudited)\n  terminals: %d; execution workers: %d\n"+
		"  mix: %t; response: %t; deferred Delivery: %t; 120-minute measurement: %t\n"+
		"  Delivery results: %s\n  %s\n",
		r.Terminals, r.ExecutionWorkers, r.MixPass, r.ResponsePass,
		r.DeliveryPass, r.MeasurementPass, r.DeliveryResults, r.Note)
}
