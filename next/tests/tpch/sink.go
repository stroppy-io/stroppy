package main

// QuerySink construction + the latency formatter shared with tpcc's MixSink
// (kept here so the tpch domain sink stays self-contained).

import (
	"fmt"
	"io"
)

// newQuerySink builds a QuerySink writing its summary to w.
func newQuerySink(w io.Writer) *QuerySink { return &QuerySink{w: w} }

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
