package main

// The TPC-H workload: a single sequential pass over the 22 business queries
// (§2.4), each executed once with its spec-pinned substitution parameters, and
// each timed into the per-query servicetime histogram declared in Define (D6:
// no print side-channel — the QuerySink renders the breakdown from the Report).
//
// The step runs under the Once executor: TPC-H queries are heavy analytic
// passes, not a closed-loop OLTP mix, so "one round of q1..q22" is the natural
// power run (matching v5's runTpchQueries). A future closed/open loop over the
// query set is a magnitude change on the step, not a body change (D4).

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/metrics"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// workloadHandler runs q1..q22 once each, recording per-query servicetime into
// lat and per-query run/error counts. The 22 prepared handles are warmed in Init
// (vu.Prepare caches them) so the single Iter binds and executes on cache hits.
type workloadHandler struct {
	file *sqlfile.File
	lat  *bench.Histogram
	runs *bench.Counter
	errs *bench.Counter
}

type workloadState struct {
	stmts map[string] /*query name*/ *sqlfile.Query
}

func (h *workloadHandler) Init(vu *bench.VU) error {
	st := bench.Local[workloadState](vu)
	if _, err := vu.Conn(); err != nil {
		return err
	}
	st.stmts = make(map[string]*sqlfile.Query, len(queryNames))
	for _, name := range queryNames {
		q, ok := h.file.Query(name, "body")
		if !ok {
			return fmt.Errorf("tpch: query %s/body missing from SQL corpus", name)
		}
		// Warm the prepare cache now (plan phase) so the Iter path is cache hits.
		if _, err := vu.Prepare(q); err != nil {
			return fmt.Errorf("tpch: prepare %s: %w", name, err)
		}
		st.stmts[name] = q
	}
	return nil
}

func (h *workloadHandler) Iter(vu *bench.VU) error {
	st := bench.Local[workloadState](vu)
	conn, err := vu.Conn()
	if err != nil {
		return err
	}
	var failed []string
	for _, name := range queryNames {
		stmt, err := vu.Prepare(st.stmts[name])
		if err != nil {
			h.record(vu, name, 0, true)
			failed = append(failed, name)
			log.Printf("[tpch] %s: prepare error: %v", name, err)
			continue
		}
		args := stmt.Bind()
		for _, p := range queryParams[name] {
			p.set(args)
		}
		start := time.Now()
		rows, qerr := conn.QueryWithArgs(vu.Ctx(), stmt, args)
		if qerr == nil {
			for rows.Next() {
			}
			qerr = rows.Err()
			rows.Close()
		}
		h.record(vu, name, time.Since(start).Nanoseconds(), qerr != nil)
		if qerr != nil {
			failed = append(failed, name)
			log.Printf("[tpch] %s: %v", name, qerr)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("tpch: %d query/queries failed: %v", len(failed), failed)
	}
	return nil
}

// record files one query's outcome into the per-query instruments. servicetime
// is recorded on every attempt (a failed query still took time); the runs
// counter ticks once per attempt and the errors counter once per failure.
func (h *workloadHandler) record(vu *bench.VU, name string, serviceNs int64, errored bool) {
	vu.Inc(h.runs.For(name))
	if serviceNs > 0 {
		vu.M(h.lat.For(name), serviceNs)
	}
	if errored {
		vu.Inc(h.errs.For(name))
	}
}

func (*workloadHandler) Close(*bench.VU) error { return nil }

// instrument names declared in Define; shared with the QuerySink so both
// reference one constant.
const (
	servicetimeInst = "servicetime"
	queryRunsInst   = "query_runs"
	queryErrorsInst = "query_errors"
)

// QuerySink is the TPC-H domain sink (D6): it reads the final Report's
// query-tagged instruments — servicetime histograms and the runs/errors counters
// — and formats the per-query timing table from the telemetry substrate. This
// replaces the v5 print side-channel (handleSummary) with one Sink in the chain.
type QuerySink struct {
	w io.Writer
}

func (s *QuerySink) Interval(*metrics.Report) {} // mix is a terminal summary

// Summary prints the per-query servicetime breakdown (runs, errors, p50/p95/p99)
// from the final cumulative report.
func (s *QuerySink) Summary(rep *metrics.Report) {
	runs := map[string]int64{}
	errs := map[string]int64{}
	for i := range rep.Counters {
		ct := &rep.Counters[i]
		switch ct.Inst.Name {
		case queryRunsInst:
			if ct.Inst.Tx != "" {
				runs[ct.Inst.Tx] = ct.Count
			}
		case queryErrorsInst:
			if ct.Inst.Tx != "" {
				errs[ct.Inst.Tx] = ct.Count
			}
		}
	}
	lat := map[string]*metrics.HistogramStat{}
	for i := range rep.Histograms {
		h := &rep.Histograms[i]
		if h.Inst.Name == servicetimeInst && h.Inst.Tx != "" {
			lat[h.Inst.Tx] = h
		}
	}
	fmt.Fprintf(s.w, "\n=== tpch query timings ===\n")
	var totalRuns, totalErrs int64
	for _, name := range queryNames {
		n := runs[name]
		e := errs[name]
		totalRuns += n
		totalErrs += e
		if h := lat[name]; h != nil && h.Count > 0 {
			fmt.Fprintf(s.w, "  %s  runs=%d  errors=%d  p50=%s p95=%s p99=%s  max=%s\n",
				name, n, e, fmtDur(h.P50), fmtDur(h.P95), fmtDur(h.P99), fmtDur(h.Max))
		} else {
			fmt.Fprintf(s.w, "  %s  runs=%d  errors=%d\n", name, n, e)
		}
	}
	fmt.Fprintf(s.w, "  total  runs=%d  errors=%d\n", totalRuns, totalErrs)
}
