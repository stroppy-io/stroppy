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

# Keep JSON output without saving history
stroppy run tpcb/tx -d pg --no-save-report --report-format json

# Disable report construction
stroppy run simple -d noop --no-report
```

`--report-format` currently accepts `json`. `--report-file` publishes the file
atomically. A requested output failure returns a nonzero exit status. Every
constructed report is also saved under `~/.stroppy/reports/`; `--no-save-report`
disables only automatic history, while `--no-report` disables both report
construction and history. History failures warn
on stderr without failing the run. When a run itself fails after reporting starts,
Stroppy still writes the requested report with `status: "failed"` or
`status: "canceled"`, then returns the run error.

## Saved report history

The installed and standalone CLIs expose the same history commands:

```bash
stroppy reports list
stroppy reports list --workload tpcc/tx --driver pg --status completed --limit 10
stroppy reports list --since 2026-10-01 --until 2026-10-14 -o json
stroppy reports get 2026-10-13T09-15-00Z-tpcc-tx > result.json
stroppy reports status -o json

# Removal and cleanup preview by default; --yes authorizes deletion.
stroppy reports remove 2026-10-13T09-15-00Z-tpcc-tx
stroppy reports remove 2026-10-13T09-15-00Z-tpcc-tx --yes
stroppy reports prune --older-than 720h
stroppy reports prune --older-than 720h --workload tpcc/tx --yes
```

History IDs are filenames without `.json`: UTC run start at second precision,
normalized lowercase workload name, and an optional collision suffix (`-2`,
`-3`, and so on). These IDs are separate from the report's `id` and `run_id`,
which may be user-supplied and reused. Existing history files keep their IDs;
there is no migration. `get` returns the stored JSON unchanged, including unknown
fields and workload payloads. Files are published only after their contents are
written, so concurrent readers do not see partly written reports.

`list` defaults to the newest 20 reports. `--limit 0` returns all results;
`--reverse` orders oldest first. Ordering uses `started_at`, then history ID,
never filesystem modification time. Filters run before sorting and limiting:

- `--workload` matches exact names, including custom workloads.
- `--driver` matches any named driver's type; `pg` and `pico` are aliases for
  `postgres` and `picodata`.
- `--status` accepts `completed`, `completed_with_errors`, `failed`, or `canceled`.
- `--since` includes the boundary; `--until` excludes it. Both accept RFC3339
  timestamps or `YYYY-MM-DD` at midnight UTC, and compare run start times.
- Repeated or comma-separated values within one filter match any value; different
  filters must all match. For example, `--status failed,canceled --driver pg`.

Human listings show history ID, start time, workload, named drivers, status, and
native workload TPS where available. TPS is the `workload` step's native rate,
including its drain window, not TPC-C tpmC; warmup rates are not added. Missing TPS
is shown as `-`. JSON listings contain common metadata only; use `get` and tools
such as `jq` for full report inspection. `-o human|json` also applies to storage
status and removal previews/results. Diagnostics go to stderr, not JSON stdout.

`status` reports location, existence, healthy/unreadable counts, and bytes in
regular candidate report files. A missing history directory is empty and is not
created by read commands. Malformed or unreadable files produce warnings without
blocking healthy reports. Cleanup skips those files because their start time
cannot be trusted; an explicitly named corrupt regular report can still be
removed. Symlinks and arbitrary paths are rejected. Explicit batch removal
validates all targets before deleting any, but filesystem errors during deletion
can still leave a partially removed batch and return a nonzero exit status.

`prune` requires a positive Go duration such as `720h` (30 days). It selects run
starts strictly older than that age and accepts the same workload, driver,
status, and time filters. There is no automatic retention or background cleanup.
A preview is a snapshot, not a reservation; inspect again before authorizing a
later deletion if other processes are changing history.

Reports belong to the local user. Back up or copy `~/.stroppy/reports/` to retain
history; `get ID > file.json` exports one complete report. Run history does not
store driver URLs, authentication fields, or OTLP headers. Authored parameters,
metadata, and payloads must not contain credentials. `stroppy baseline` retains
its separate schema and `~/.stroppy/baselines/` history; these commands neither
list nor remove baseline reports.

## Report envelope

Common fields include report schema and identity, Stroppy version, timestamps,
host runtime facts, driver types, effective scenario and parameter values,
parameter sources, step selection and observed step results, measurement time,
metrics, bounded error groups, and terminal failure information. Schema 3 adds
named driver identities, per-step `executions` policy settings, and `measurements`
windows including actual drain. The `scenario` field is a convenience view of the
first measured step. Optional `build_digest` identifies cached generated-runtime
or exported runs; direct installed built-in runs omit it. Driver URLs,
authentication fields, and OTLP headers are not included as configuration.
Configured connection strings and credential values are also redacted from
report failure and diagnostic messages before truncation. This does not scrub
arbitrary authored parameters, metadata, or opaque workload payloads, and does
not rewrite existing reports.

Counters and gauges contain aggregate totals plus dimensioned series. Histograms
contain count, sum, average, fixed bounds, bucket counts, p50/p90/p95/p99, and
dimensioned series. Report metric types are plain JSON and do not expose
OpenTelemetry SDK types.

Workloads may attach small non-secret string fields with `d.Report.Metadata`,
or add independently versioned payloads under `workload_reports` with
`d.Report.Contribute(kind, schema, builder)` and `d.Report.Put(kind, schema, data)`.
Builders receive an independent copied final snapshot with aggregate/label-series
metrics, reached step outcomes, measurement windows, and run status. `Render`
writes human output through an explicit writer from the same encoded payload.
Do not put secrets in metadata, custom fields, or payloads.

`Application.Run` returns a report without saving history. Hosts can call
`report.Save` explicitly. `Application.Execute` and CLI entrypoints retain
automatic history behavior. `NoReport` skips builders and payload encoding, not
metrics or terminal-error accounting.

Built-in typed payloads:

- `tpcc.compliance`: transaction mix, tpmC, latency percentiles, ceilings,
  statistical sufficiency, and steadiness;
- `tpch.validation`: per-query answer status, row counts, bounded mismatches, and
  aggregate totals;
- `tpcds.validation`: same validation contract for TPC-DS queries.

Skipped or failed workload-specific reporting is explicit through each payload's
`status` and `reason`; unknown payload kinds remain valid report data.

Envelope schema changes when shared provenance or result contracts evolve. Each
workload payload carries its own schema.
