package main

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/stroppy-io/stroppy/next/metrics"
)

// MixSink is the TPC-C domain sink (D6): it reads the final Report's tx-tagged
// instruments — the per-tx tx_count counters and tx_latency histograms the test
// declares in Define — and formats the transaction-mix table with tpmC. This is
// the report the print side-channel in validate.go used to produce, now routed
// through the unified telemetry substrate so all domain reporting goes through
// one Sink chain and no test step fmt.Printf's results.
//
// Compose it with the generic ConsoleSink via metrics.MultiSink so the generic
// instrument table and this domain view both render from one Report. Interval is
// a no-op: the ConsoleSink in the chain already renders per-tick lines, and the
// mix is a terminal summary (counts and tpmC are only meaningful over the whole
// run).
type MixSink struct {
	w          io.Writer
	warehouses int64
	vus        int
}

// newMixSink builds a MixSink writing to w, capturing the run's warehouse and VU
// counts for the mix-table header (the Report carries elapsed time, not scale).
func newMixSink(w io.Writer, o *options) *MixSink {
	return &MixSink{w: w, warehouses: o.Warehouses, vus: o.VUs}
}

// Interval is a no-op; see the type doc.
func (s *MixSink) Interval(*metrics.Report) {}

// Summary prints the per-tx counts and latencies and the tpmC figure from the
// final (cumulative) report.
func (s *MixSink) Summary(rep *metrics.Report) {
	counts := map[string]int64{}
	var total int64
	for i := range rep.Counters {
		ct := &rep.Counters[i]
		if ct.Inst.Name == txCountInst && ct.Inst.Tx != "" {
			counts[ct.Inst.Tx] = ct.Count
			total += ct.Count
		}
	}
	lat := map[string]*metrics.HistogramStat{}
	for i := range rep.Histograms {
		h := &rep.Histograms[i]
		if h.Inst.Name == txLatencyInst && h.Inst.Tx != "" {
			lat[h.Inst.Tx] = h
		}
	}

	fmt.Fprintf(s.w, "\n=== tpcc transaction mix (W=%d, VUs=%d, %s) ===\n",
		s.warehouses, s.vus, rep.Elapsed.Truncate(100*time.Millisecond))
	tw := tabwriter.NewWriter(s.w, 0, 2, 2, ' ', 0)
	for _, name := range txNames {
		n := counts[name]
		if h := lat[name]; h != nil && h.Count > 0 {
			fmt.Fprintf(tw, "  %s\t%d\tcount\tp50=%s p95=%s p99=%s\n",
				name, n, fmtDur(h.P50), fmtDur(h.P95), fmtDur(h.P99))
		} else {
			fmt.Fprintf(tw, "  %s\t%d\tcount\n", name, n)
		}
	}
	fmt.Fprintf(tw, "  total\t%d\n", total)
	_ = tw.Flush()

	mins := rep.Elapsed.Minutes()
	if mins > 0 {
		fmt.Fprintf(s.w, "  tpmC (new_order/min) = %.1f\n", float64(counts[txNames[txNewOrder]])/mins)
	}
}

// fmtDur renders a nanosecond latency compactly (ns/µs/ms/s). Mirrors the
// metrics package's private formatter so this sink stays self-contained.
func fmtDur(ns int64) string {
	switch {
	case ns < 1_000:
		return fmt.Sprintf("%dns", ns)
	case ns < 1_000_000:
		return fmt.Sprintf("%.1fµs", float64(ns)/1e3)
	case ns < 1_000_000_000:
		return fmt.Sprintf("%.2fms", float64(ns)/1e6)
	default:
		return fmt.Sprintf("%.2fs", float64(ns)/1e9)
	}
}

// Instrument names declared in Define; shared with main.go so the sink and the
// declarations reference one constant.
const (
	txCountInst   = "tx_count"
	txLatencyInst = "tx_latency"
)
