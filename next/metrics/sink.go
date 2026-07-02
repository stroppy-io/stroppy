package metrics

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

// HistogramStat is a snapshot of one histogram instrument for a report.
// For interval reports the figures are per-interval (delta); for the final
// summary they are cumulative over the whole run.
type HistogramStat struct {
	Inst  Instrument
	Count int64   // observations in the window (interval) or total (summary)
	Rate  float64 // observations per second
	P50   int64   // nanoseconds
	P95   int64
	P99   int64
	Max   int64
	Sum   int64 // nanoseconds; exact only in the summary
}

// CounterStat is a snapshot of one counter instrument for a report.
type CounterStat struct {
	Inst  Instrument
	Count int64   // cumulative total
	Delta int64   // change over the interval (== Count in the summary)
	Rate  float64 // Delta per second
}

// Report is the data handed to a [Sink]. Slices are reused across ticks by the
// [Reporter]; a sink must consume them synchronously and not retain them.
type Report struct {
	Elapsed    time.Duration // since the reporter started
	Window     time.Duration // length of this interval (== Elapsed for the summary)
	Final      bool          // true for the post-stop exact summary
	Histograms []HistogramStat
	Counters   []CounterStat
}

// Sink consumes reports. Interval is called on each tick with per-interval
// figures; Summary is called once, after all writers stop, with exact
// cumulative figures. Implementations must not retain the [Report] or its
// slices. The interface exists so OTel or a results store can be added later.
type Sink interface {
	Interval(rep *Report)
	Summary(rep *Report)
}

// ConsoleSink writes a compact one-line-per-active-instrument interval view and
// a final summary table to an [io.Writer].
type ConsoleSink struct {
	w io.Writer
}

// NewConsoleSink returns a console sink writing to w.
func NewConsoleSink(w io.Writer) *ConsoleSink { return &ConsoleSink{w: w} }

// Interval prints one line per histogram that saw activity this window.
func (c *ConsoleSink) Interval(rep *Report) {
	el := rep.Elapsed.Truncate(time.Second)
	for i := range rep.Histograms {
		h := &rep.Histograms[i]
		if h.Count == 0 {
			continue
		}
		line := fmt.Sprintf("[%6s] %-24s n=%-8d %8.0f/s  p50=%s p95=%s p99=%s",
			el, label(h.Inst), h.Count, h.Rate,
			formatDur(h.P50), formatDur(h.P95), formatDur(h.P99))
		if e := c.errRate(rep, h.Inst.Step); e >= 0 {
			line += fmt.Sprintf("  err=%.0f/s", e)
		}
		_, _ = io.WriteString(c.w, line+"\n")
	}
}

// errRate returns the per-second rate of a counter named "errors" for the given
// step, or -1 when there is none.
func (c *ConsoleSink) errRate(rep *Report, step string) float64 {
	for i := range rep.Counters {
		ct := &rep.Counters[i]
		if ct.Inst.Name == "errors" && ct.Inst.Step == step {
			return ct.Rate
		}
	}
	return -1
}

// Summary prints the final per-instrument table.
func (c *ConsoleSink) Summary(rep *Report) {
	_, _ = fmt.Fprintf(c.w, "\n=== summary (%s) ===\n", rep.Elapsed.Truncate(time.Millisecond))
	tw := tabwriter.NewWriter(c.w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "instrument\tcount\trate/s\tp50\tp95\tp99\tmax")
	for i := range rep.Histograms {
		h := &rep.Histograms[i]
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%.1f\t%s\t%s\t%s\t%s\n",
			label(h.Inst), h.Count, h.Rate,
			formatDur(h.P50), formatDur(h.P95), formatDur(h.P99), formatDur(h.Max))
	}
	_ = tw.Flush()
	if len(rep.Counters) > 0 {
		ctw := tabwriter.NewWriter(c.w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(ctw, "counter\ttotal\trate/s")
		for i := range rep.Counters {
			ct := &rep.Counters[i]
			_, _ = fmt.Fprintf(ctw, "%s\t%d\t%.1f\n", label(ct.Inst), ct.Count, ct.Rate)
		}
		_ = ctw.Flush()
	}
}

// label renders an instrument as a compact "step/name" key, appending tx/table
// when present.
func label(inst Instrument) string {
	s := inst.Name
	if inst.Step != "" {
		s = inst.Step + "/" + inst.Name
	}
	switch {
	case inst.Tx != "":
		s += "[" + inst.Tx + "]"
	case inst.Table != "":
		s += "[" + inst.Table + "]"
	}
	return s
}

// formatDur renders a nanosecond duration compactly (ns/µs/ms/s).
func formatDur(ns int64) string {
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
