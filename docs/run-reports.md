# Run reports

`stroppy run` builds a versioned machine-readable report for every registered
workload. Human summaries and diagnostics remain on stderr. JSON is written only
when requested, so stdout contains one complete document and no log lines.

```bash
# JSON on stdout
stroppy run tpcb/tx -d pg --report-format json

# Same document in a file
stroppy run tpcc/tx -d pg --report-file result.json

# Both destinations
stroppy run simple -d noop --report-format json --report-file result.json

# Disable report construction
stroppy run simple -d noop --no-report
```

`--report-format` currently accepts `json`. `--report-file` publishes the file
atomically. A requested output failure returns a nonzero exit status. When a run
itself fails after reporting starts, Stroppy still writes the requested report
with `status: "failed"` or `status: "canceled"`, then returns the run error.

Common fields include report schema and identity, Stroppy version, timestamps,
host runtime facts, driver types, effective scenario and parameter values,
parameter sources, step selection and observed step results, measurement time,
metrics, bounded error groups, and terminal failure information. Driver URLs,
authentication fields, and OTLP headers are never included.

Counters and gauges contain aggregate totals plus dimensioned series. Histograms
contain count, sum, average, fixed bounds, bucket counts, p50/p90/p95/p99, and
dimensioned series. Report metric types are plain JSON and do not expose
OpenTelemetry SDK types.

Workloads may attach small custom string fields with `Bench.AddReportData`, or add
independently versioned typed payloads under `workload_reports` through
`Def.Report`. Custom fields belong under `custom`; callers must not put secrets
there.

Built-in typed payloads:

- `tpcc.compliance`: transaction mix, tpmC, latency percentiles, ceilings,
  statistical sufficiency, and steadiness;
- `tpch.validation`: per-query answer status, row counts, bounded mismatches, and
  aggregate totals;
- `tpcds.validation`: same validation contract for TPC-DS queries.

Skipped or failed workload-specific reporting is explicit through each payload's
`status` and `reason`; unknown payload kinds remain valid report data.

Envelope schema changes only for breaking contract changes. Additive optional
fields retain current schema. Each workload payload carries its own schema.
