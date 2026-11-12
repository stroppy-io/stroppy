# Client connection-pool diagnostics

Stroppy reports connection-pool pressure separately from database execution.
The startup warning remains advisory: workers above a finite connection cap can
queue, but a warning is not evidence that any acquisition waited.

## Coverage

- PostgreSQL, CockroachDB, and YugabyteDB expose their pgx pool statistics.
- MySQL and TiDB expose their main `database/sql` pool statistics. Separate
  backend pools used by isolated routines are not observed; main-pool zero wait
  does not establish that routine execution is unconstrained.
- YDB exposes only its SQL pool. Native BulkUpsert and SDK session pools are not
  observed; zero SQL wait does not establish that a native load is unconstrained.
- Picodata's pinned SDK does not expose its per-instance pgx statistics. Its pool
  diagnostics are explicitly unavailable, not zero.
- Noop, CSV, recording, and other drivers without this optional capability report
  unavailable telemetry when used.

No pool is resized and no additional connection acquisition is introduced.

## Gauges

Each used database and step has `pool_stats_available` (1 or 0). Supported pools
also expose `pool_connections_open`, `pool_connections_in_use`,
`pool_connections_idle`, and `pool_connections_max`. Open includes connections
occupying capacity while opening. Maximum 0 means unlimited; -1 means unknown.

These are pool-wide snapshots, not counts of active server queries. Gauge values
freeze at the step's final sample, so a final in-use value of zero does not mean
the pool was never saturated. Enable metric capture or OTLP for evolution over
time.

## Native wait counters

The backend definitions differ and deliberately have different names:

| Metrics | Meaning |
|---|---|
| `pool_sql_capacity_waits_total`, `pool_sql_capacity_wait_seconds_total` | `database/sql` requests waiting at the maximum-open limit; includes canceled waits; excludes directly opening a connection below the limit. |
| `pool_pgx_empty_acquires_total`, `pool_pgx_empty_acquire_seconds_total` | Successful pgx acquisitions that found no resource available; includes new-connection construction; excludes canceled waits. |
| `pool_pgx_canceled_acquires_total` | pgx acquisition attempts canceled by context; their duration is not included in successful acquisition time. |

These counters are not interchangeable queue-latency measurements. In particular,
pgx `AcquireDuration` is not exported as pool wait: it includes every successful
acquisition, even when the pool was not empty.

Counters are deltas from the first observation in each step. Existing pools are
baselined before worker initialization; newly opened drivers are baselined after
connection setup and before the first workload operation. Worker initialization
and actual drain are included, and the final sample precedes driver teardown.
Concurrent workers' accumulated wait seconds can exceed wall-clock seconds.

Metric attributes are `step`, `step_execution`, `database`, `driver`,
`pool_backend`, and `pool_scope`, plus the normal run identity. The execution
ordinal distinguishes repeated invocations of the same named step. No endpoints or credentials are
included. OTLP adds the normal configured metric prefix (default `stroppy_`).

## Reports and dashboards

JSON reports add an optional `pools` array with one final observation per used
database and step. Unsupported measurements omit numeric values rather than
filling them with zero. Reports without this field remain readable. Pool gauges
have only labeled series, not a run-wide total: snapshots from different steps
must not be added into an invented pool capacity. Text summaries print those
series separately, and aggregate-only workload snapshots omit pool gauges.

The local HTML report has a **Connection pools** section explaining coverage and
native counter semantics. Exact metric series remain available under Metrics;
metric capture can display their timelines.

Import `docs/dashboard.json` for Grafana's **Client connection pools** panels.
Select a run/instance, database, and step. Pool panels do not use transaction,
query, or scenario filters because these are pool-level observations. SQL and
pgx waits remain separate panels, with cumulative per-step values rather than
a misleading common wait percentage.
