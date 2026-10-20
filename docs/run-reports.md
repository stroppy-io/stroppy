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

## HTML reports and local viewer

```bash
# One self-contained HTML file, generated during a run
stroppy run tpcc/tx -d pg --report-html result.html

# JSON and HTML can be requested together, with or without history saving
stroppy run tpcc/tx -d pg --no-save-report \
  --report-format json --report-html result.html

# Export an existing report by its history ID
stroppy reports export 2026-10-13T09-15-00Z-tpcc-tx --file result.html
stroppy reports export 2026-10-13T09-15-00Z-tpcc-tx > result.html

# Browse saved reports; the actual loopback URL is printed
stroppy reports serve
stroppy reports serve --listen 127.0.0.1:8080
```

The same Go renderer powers run output, saved-report export, and the local server.
Exported HTML includes its stylesheet, SVG charts, complete original JSON, and an
optional bundled chart-enhancement script. There are no CDN assets, external fonts,
or frontend build/runtime requirements.
Open the file directly in a browser. Native expandable sections, numeric tables,
chart hover/focus details, anchors, and JSON download work without JavaScript.

Catalog, run, capture, and comparison pages share the vendored Stroppy kit 0.1.0
palette, component styles, mark, Inter text, and Roboto Mono numeric/code fonts.
Fonts and styles are embedded in each HTML export, including portable bundle
previews; no network request or runtime asset directory is needed. Theme follows
the system light/dark preference, with a light print palette. Existing SVG charts
use the kit series colors while keeping dash patterns, numeric tables, and
script-free hover/focus. With JavaScript available, kit charts enhance bar plots
and captured timelines, using numeric elapsed-time axes, straight lines, explicit
gaps, and phase markers. The SVG stays visible until a successful mount; blocked
scripts or initialization failures leave it usable. Each enhanced plot has a
static/interactive switch, numeric tables remain available, and print uses SVG.
No resampling, smoothing, or new benchmark calculations are introduced.
The server allows only the exact bundled script by CSP hash. The pinned release
source, archive checksum, and font/uPlot licenses live under
`pkg/report/view/kit/`.

The report starts with a compact overview and a collapsed identity/configuration
section. Expand it to inspect timestamps, report/run IDs, build/runtime identity,
parameter values and sources, driver identities, metadata, execution policies,
and unknown envelope fields. Nested objects use expandable key/value views;
arrays of objects become tables with identity columns first when their shape is
bounded. Sparse or overly wide arrays use an expandable list instead, avoiding
quadratic empty-cell allocation while keeping every authored value. Custom metrics and
workload-owned results use those same generic views, without requiring a workload
adapter. Large integer identifiers are preserved as JSON numbers, not rounded
through JavaScript floating-point conversion. Complete JSON remains downloadable
byte-for-byte from saved reports, including fields the viewer does not understand.

Metrics are grouped by type with naturally ordered names and aligned preview
columns. Distributions show count, average, and p99; counters and gauges show their
recorded aggregate value. Human-readable labels retain the exact metric key below.
Numeric previews use digit grouping, bounded decimal precision, and large-number
suffixes; `≈` marks approximation. Exact original values remain in hover titles,
expanded metric data, and the JSON download. Series charts expand independently.

Charts show recorded step durations, histogram bucket counts with overflow, and
small labeled numeric payload tables or compatible metric series. Bucket intervals
retain their reported units, and histogram bars represent counts, not probability
density or reconstructed samples. Empty finite buckets are omitted from the plot
with an explicit count, while overflow remains visible and all original buckets
remain in exact metric data. Step, phase, and outcome dimensions stay separate;
ambiguous or aggregate-only native headline values are omitted rather than summed
or assumed to cover the workload step. The catalog uses the same workload-only
TPS boundary and shows every named driver identity. Missing values
are not converted to zero. Charts are optional projections: unsuitable or large
arrays remain available in structured views, and every plotted value has a table alternative.
No throughput timeline is inferred from final aggregates. Native TPS is not TPC-C
tpmC, and the viewer does not issue audited benchmark-compliance claims.

`serve` uses an available port on `127.0.0.1` by default and does not open a browser
automatically. The catalog provides workload, driver, status, start-time, and order
filters plus 20-report pages using ordinary GET forms. Report pages and original
JSON downloads are read-only; no web route runs workloads or deletes history.
Cancel the command to stop the server; active responses get a five-second drain
before forced connection closure. `--listen` can explicitly select another
interface; there is no authentication or multi-user hosting contract, so use it
only in the intended trusted environment.

Malformed entries warn in the catalog without hiding healthy reports. The inherited
32 MiB metadata-inspection limit still applies to catalog discovery; explicit
export or viewing by ID reads the full report. Unfamiliar envelope schemas display
a warning and recognizable shared fields plus complete original data. Report text
is HTML-escaped, including authored SQL, logs, and payload strings.

`--report-html` does not change `--report-format` or `--report-file`; JSON and HTML
paths must differ. `--no-report` conflicts with any requested output, including
HTML, while `--no-save-report` leaves HTML available. HTML files are published
atomically, and a requested write/rendering failure returns a nonzero exit status.
Filesystem aliases, including symlinked parent directories, cannot select the
same JSON/HTML destination; HTML export also refuses to replace its source history
entry. Case variants are distinct when the destination filesystem is case-sensitive;
for absent outputs, a temporary probe in their parent directory checks this behavior.
If stdout fails, independently requested file outputs are still attempted.

Explicit HTML file exports use owner-only mode `0600`, matching JSON report files.
For sharing through another user or web server, change permissions explicitly after
export (for example, `chmod 644 result.html`). Atomic publication replaces an output
symlink itself rather than writing through it; the symlink target remains unchanged.
Shell redirection (`reports export ID > result.html`) uses the shell's usual file
creation and permission rules.

## Optional log and metric recording

Final reports always retain the existing metric summary. Historical samples and
structured logs are separate, independently enabled recordings:

```bash
stroppy run tpcc/tx -d pg --capture-metrics
stroppy run tpcc/tx -d pg --capture-logs
stroppy run tpcc/tx -d pg --capture-logs --capture-metrics \
  --capture-interval 1s --capture-max-bytes 67108864

# Explicit output without history saving
stroppy run tpcb/tx -d noop --capture-metrics --no-save-report \
  --report-file run.json --report-html run.html

# Retrieve complete append-only streams or a portable rich-report ZIP
stroppy reports artifact ID metrics.jsonl > metrics.jsonl
stroppy reports artifact ID logs.jsonl > logs.jsonl
stroppy reports bundle ID --file run.zip
```

Both recording flags default off. `--capture-interval` defaults to one second and
is independent of the OTLP export interval. `--capture-max-bytes` is a per-stream
byte budget (64 MiB by default, at least 256 KiB). `--capture-dir` selects a
persistent recording root; every invocation creates a fresh UUID subdirectory,
even when `run_id` is reused. Capture-only settings without an enabled stream are
rejected. `--no-report` conflicts with capture. With `--no-save-report`, supply
`--capture-dir`, `--report-file`, or `--report-html` so recording has a persistent
location. Final JSON remains small: schema 4 adds only the optional versioned
`capture` manifest, not the potentially large streams.

### Recording semantics and correlation

Metric samples retain names, units, public dimensions, cumulative counters and
histogram buckets/count/sum, and gauges as recorded. Collection uses the same
manual reader and public projection as the final report, after merging private
worker series. The sampler stops and joins before final report collection; one
last snapshot is recorded after execution. Local capture does not require OTLP,
and capture plus OTLP export remains supported. Native TPS and query/iteration
rate gauges are cumulative averages, **not instantaneous rates**. Histograms are
not raw observations. The final report remains the authoritative result.

Logs contain timestamp, elapsed time, effective level, logger name, message,
structured fields, and available step/worker context. Log capture tees the
run-owned logger and preserves existing stderr output and effective log level;
it does not enable debug logging. Workload, framework, and driver logs routed
through that logger are eligible. Arbitrary stdout/stderr writes, unrelated
process logs, database-server logs, profiles, and host telemetry are not captured.
Transaction/iteration identities are retained when authored in fields; absent
context is not inferred. Capture and report UUIDs correlate streams, while
`source_id` retains the original capture identity if history makes a copy.

An event stream records run and execution boundaries, skipped/blocked steps,
configured worker/duration budgets, actual step completion, drain entry, and
teardown. Timestamped metric dimensions and those events distinguish setup,
warmup, measurement, drain, and teardown where those phases were recorded.

### Limits, failures, and interrupted runs

Each stream has its own bounded queue and writer. Workload log calls do not wait
for file I/O. A full queue drops a capture record, not a benchmark operation.
Records above 256 KiB are omitted and counted as truncated. Logs cannot consume
metric/event queue capacity or byte budgets; events have a separate 1 MiB budget.
When a stream reaches its budget or writing fails, it stops writing. The manifest
records bytes, records, drops, truncation, missed samples, and write errors.
Recording state becomes `incomplete` or `failed` rather than claiming completeness.
Capture warnings do not turn successful database transactions into failures.
Invalid destinations fail before execution. Existing query/retry/error semantics
are unchanged.

Sampling never builds an unbounded backlog: only one collection is active, and
missed ticks are counted. Failed collection and skipped sequence numbers expose
metric gaps. Graceful cancellation or workload failure retains available data and
finalizes its capture. A hard interruption leaves a `recording` manifest and
append-only JSONL streams; complete records remain inspectable, and the viewer
marks recording incomplete. A partly written final line is omitted with a visible
warning. A crash before final report saving can leave an unattached UUID directory
in the recording root; retain it for recovery or remove it manually after checking
its manifest. No report is invented for a run whose final envelope was never saved.

### History, rich viewing, and export

History stores exclusively owned capture copies under
`~/.stroppy/reports/captures/<uuid>/`. The saved report uses a relative reference;
repeated saves have separate storage identities. Listing exposes `capture_state`.
Explicit artifacts, removal, and age-based pruning resolve only owned UUIDs and
declared regular files; symlink and traversal targets are rejected. Removing one
report does not remove another report's recording. Default staging is removed
after a successful history-only save; explicitly selected roots and captures
referenced by requested output remain caller-owned. Copy the report and its
relative `captures/` subtree together when backing up history.

The web report adds metric evolution, per-series numeric samples, execution event
inspection, and a bounded log list. Log level, step, elapsed range, and text filters
use ordinary GET forms. Selecting a metric works for custom instruments too;
histograms show cumulative averages and observation counts on separate plots.
Timeline series keep dimensions
separate, label cumulative/gauge semantics, and break lines across missed samples.
Phase markers and numeric-sample links connect to nearby logs. Missing values are
not interpolated. Large plots show a bounded selection of points with an omission
notice instead of pretending omitted data is continuous.

HTML remains usable without JavaScript. Standalone HTML includes a bounded
preview, while `reports bundle` retains `index.html`, portable `report.json`, the
capture manifest, and **complete** original streams. Extract the ZIP and open
`index.html` offline; artifact links are relative. The viewer inspects at most
8 MiB and 2,000 records per metric/event preview and shows 100 filtered logs per
page within its bounded scan. Logs use a height-limited, scrollable list of
collapsed records: elapsed time, level, and message are visible; expand a record
for its full message, timestamp, logger, and structured fields. A Logs navigation
link jumps directly to this list. It never silently labels a preview as the full
recording. Download the streams when more data is needed. Ordinary reports remain
valid without capture and require no sidecar files.

Rich recordings may contain whatever the workload or driver logs, including
sensitive authored data. Existing diagnostic redaction remains in place, but
arbitrary captured fields are not promised secret-free. Log text is HTML-escaped.
Explicit capture and bundle files are owner-readable; share permissions deliberately.

Programmatic callers set `bench.RunOptions.Capture` or `stroppy.RunRequest.Capture`
with `bench.CaptureOptions`. They must choose a persistent directory and reporting
options. Programmatic execution returns the manifest and never silently saves
history; call `report.Save` explicitly to attach the recording to local history.

### Local overhead evidence

Measured on Apple M5 Pro, macOS/arm64, Go 1.27.2 with a single worker, a custom
counter/log call per logical operation, logs enabled to a discard writer, and
three repetitions. These are framework-only measurements, not database TPS.

```bash
go test ./pkg/bench -run '^$' -bench '^BenchmarkCaptureOverhead$' \
  -benchmem -benchtime=200000x -count=3
```

Median fixed-operation costs were approximately 596 ns/op with capture off,
661 ns/op with metrics only, 1,483 ns/op with logs only, and 1,754 ns/op with both.
Each capture-on run used a one-second interval and the default byte budgets.
These short runs produced initial/final metric snapshots only. Log pressure
produced approximately 43,000–92,000 dropped records per 200,000 operations,
explicitly accounted in manifests; lossless capture is not claimed. These samples
were collected while repository validation was active, so scheduling and writer
contention contribute to the observed variation.

Two-second runs produced three or four metric samples and measured about
580 ns/op off versus 589 ns/op metrics-only. Log-enabled runs reached the 64 MiB
budget, stopped detailed encoding, and therefore had lower average per-operation
cost than the unsaturated fixed-operation case; those runs are not evidence of
sustained lossless log performance. Collection cost grows with metric cardinality,
and structured field encoding remains on the logging call path. Choose sampling,
log level, and budgets to suit the benchmark, and inspect capture accounting.

## Comparing saved runs

```bash
stroppy reports compare ID_A ID_B
stroppy reports compare ID_A ID_B ID_C --reference ID_B -o json
stroppy reports compare ID_A ID_B -o html --file comparison.html
```

Select 2–8 distinct history IDs. Input order is retained; the first run is the
reference unless `--reference` selects another input. `-o human|json|html` uses one
comparison model. `--file` atomically publishes any format and refuses to replace
selected source reports or capture artifacts. JSON contains calculated values,
unavailability reasons, configuration/result differences, and complete original
report documents. Source numbers and opaque workload payloads remain intact.

The catalog supports report checkboxes, filtering, and pagination without
JavaScript. Filter/page submissions retain selections, including reports outside
the visible page. Compare selected reports, choose a reference, and share the
local `/compare?id=...&id=...` URL. Source reports and per-run logs remain linked.
Export comparison HTML from that page for an offline snapshot with embedded source
JSON; local-server links and filtering controls are omitted from offline output.

Metrics match by name, type, recorded unit, complete dimensions, and statistic.
Missing values are unavailable, not zero. An empty recorded unit means unspecified;
no unit conversion is attempted. Aggregate-only values never match dimensioned
series, and aggregate deltas require matching recorded step scopes. Warmup,
measurement, transaction type, outcome, and other dimensions remain separate.
Unit/type/scope mismatches retain source values with unavailable comparison cells.

Absolute change is candidate minus reference. Percentage change is
`100 * (candidate - reference) / abs(reference)`; a zero reference has no percentage.
Decimal subtraction preserves exact source numbers. Repeating percentage decimals
are rounded to 12 places and flagged approximate; display previews may round
further, with exact values in JSON. A positive change is not an improvement verdict.
Native TPS is not TPC-C tpmC. No significance or benchmark-compliance claim is made.

Histogram buckets compare only with identical recorded boundaries. Percentiles
are not averaged across series. Recorded aggregate percentiles are comparable only
in their unambiguous scope. Per-series `bucket_p50/p90/p95/p99` estimates use linear
interpolation within a finite bucket, explicitly marked approximate; quantiles in
an unbounded first/overflow bucket are unavailable. Histograms retain count, sum,
average, and original buckets for inspection. Comparison plots combine average,
median/p50, p90, p95, and p99 on one same-unit axis, with adjacent run bars.
Count and sum plots remain separate. Run colors stay consistent across charts,
including static fallbacks, and missing values keep their run slot without
becoming zero bars. Unknown metric types and report schemas remain inspectable without guessed numeric comparisons. Workload-owned
results receive generic structural differences, not inferred units or adapters.

Configuration/result differences include parameters and sources, named drivers,
versions/build IDs, runtime facts, execution policies, measurement/drain windows,
status, failures, retries, and workload payload schemas/data. Deliberate cross-host
or cross-driver experiments remain allowed. Matching recorded inputs is not proof
of equivalent database hardware/settings or identical workload source.

Capture overlays use elapsed time since run start, never inferred measurement
alignment. Only matching metric/unit/dimension/statistic series share a plot.
Dash patterns and run labels distinguish sources; numeric tables retain samples.
Phase markers, sequence/time gaps, sampling intervals, omissions, and incomplete
capture notices remain visible. No resampling, interpolation between samples, or
derived instantaneous rate is applied. Missing captures do not block comparison.
Offline HTML embeds bounded timeline previews, not complete capture sidecars;
retrieve each source run's bundle for complete recordings.

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
written, so concurrent readers do not see partly written reports. On filesystems
without hard links, publication uses a reserved filename and atomic rename.
Interrupted publication can leave hidden `.report-*` or `.publish-*` artifacts;
read commands ignore them and later saves use a different collision suffix.

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
blocking healthy reports. Metadata inspection is bounded to 32 MiB per report;
larger entries warn and are skipped by list/status/prune, while `get` still
retrieves them in full. Cleanup skips unreadable entries because their start
time cannot be trusted; an explicitly named corrupt or oversized regular report
can still be removed. Symlinks and arbitrary paths are rejected. Explicit batch removal
validates all targets before deleting any, pins the selected directory and file
identities, and rejects replacements. Filesystem errors during deletion can
still leave a partially removed batch and return a nonzero exit status. If a
replacement is detected after staging, it is retained rather than deleted at the reported
`.remove-*` recovery location.

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
report failure and diagnostic messages before truncation. Built-in TPC-H/TPC-DS
validation diagnostics use the same boundary; custom contributors can call
`snapshot.Diagnostic(message)` for diagnostic fields inside their payloads.
This does not scrub arbitrary authored parameters, metadata, or opaque workload payloads, and does
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
