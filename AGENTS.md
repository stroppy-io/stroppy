# Stroppy — Agent Context

Database stress testing CLI powered by k6. Apache 2.0.

## Changelog

`CHANGELOG.md` feeds the docs site — write plain-English one-liners for *users* (not commit-speak), grouped under `## [Unreleased]` by Added/Changed/Fixed. For any user-facing change, add the line **in the same commit** as the change (atomic) — propose it with the diff, don't chase it afterward; for direct/no-PR commits leave it plain text. When the change goes through a PR, after the PR is opened add a follow-up commit that appends the correct link `([#NN](https://github.com/stroppy-io/stroppy/pull/NN))` to that line.

## Binary Layout

`make build` produces ONE binary: xk6 builds `build/k6` with stroppy embedded (`--with cmd/xk6air`), then `build/stroppy` is `cp build/k6 build/stroppy`. Both names are the same file; dispatch is strictly by name — invoked as `stroppy` it strips the `k6 x stroppy` prefix, any other name acts as k6 + extension.

## Build & Lint

```
make build          # ALWAYS use this — never go build ./...
make linter_fix     # run first, auto-fixes formatting
make linter         # read-only check after linter_fix
make tests          # all tests with race detector and coverage
make proto          # regenerate Go/TS/docs from .proto; wipes pkg/common/proto/* — never hand-edit generated files
make ts-test        # TypeScript unit tests
make ts-typecheck   # typecheck helpers.ts / datagen.ts / parse_sql.ts / stroppy.d.ts
```

**Embedded FS rebuild rule:** `workloads/` is `//go:embed *` — if you pass a workload by short name (`tpcc/tx`, `tpcb/procs`), the binary serves from its embedded snapshot. Edits to `workloads/` on disk have **no effect** until `make build` reruns.

**Local path bypass:** If you pass an explicit local path (`./workloads/tpcc/tx.ts`, `./workloads/tpcc/pg.sql`), the runner resolves from cwd **first** — no rebuild needed. Use this during the edit-run loop:
```bash
./build/stroppy run ./workloads/tpcc/tx.ts ./workloads/tpcc/pg.sql -d pg -D url=postgres://...
```

Resolution order: **cwd → `~/.stroppy/` → embedded**.

## Directory Map

| Path | Role |
|------|------|
| `cmd/stroppy/commands/` | cobra CLI subcommands: gen, run, probe, version |
| `cmd/xk6air/` | k6 extension entry; registers `k6/x/stroppy`, manages per-VU instances |
| `pkg/driver/dispatcher.go` | driver registry: `RegisterDriver()` + `Dispatch()` |
| `pkg/driver/{postgres,mysql,picodata,ydb,noop,csv}/` | driver implementations |
| `pkg/driver/sqldriver/` | shared sql.DB-backed base (mysql, ydb use this) |
| `pkg/datagen/` | relational data-generation runtime: compile, expr, runtime, lookup, cohort, stdlib, seed |
| `internal/static/` | `helpers.ts`, `datagen.ts`, `parse_sql.ts`, generated TS type bindings |
| `internal/runner/` | esbuild transpilation, config extraction via Sobek, k6 process management |
| `proto/stroppy/` | protobuf schemas (config, run, descriptor, datagen, common, runtime) |
| `workloads/` | embedded workloads: simple, tpcb, tpcc, tpch, tpcds, execute_sql |
| `docs/datagen-framework.md` | authoritative relational datagen guide |
| `docs/parallelism.md` | InsertSpec parallelism contract and tuning |

## Drivers

| Preset | Type enum | Notes |
|--------|-----------|-------|
| `pg` | DRIVER_TYPE_POSTGRES | pgxpool-based; supports plain_query, plain_bulk, native (COPY) |
| `mysql` | DRIVER_TYPE_MYSQL | sql.DB-backed via sqldriver |
| `pico` | DRIVER_TYPE_PICODATA | sql.DB-backed; `Begin()` always errors — use isolation `"none"` |
| `ydb` | DRIVER_TYPE_YDB | sql.DB-backed; native maps to BulkUpsert |
| `noop` | DRIVER_TYPE_NOOP = 5 | discards all I/O; benchmarks stroppy/framework overhead |
| *(no preset)* | DRIVER_TYPE_CSV = 6 | URL-configured CSV output driver; InsertSpec/native-only, no query path |

CSV example:
```bash
./build/stroppy run tpcb/tx -D driverType=csv \
  -D url='/tmp/tpcb-csv?merge=true&workload=tpcb' \
  --steps drop_schema,create_schema,load_data
```

Add driver: package under `pkg/driver/<name>/`, implement `driver.Driver` (`InsertSpec`, `RunQuery`, `Begin`, `Teardown`), call `RegisterDriver()` in `init()`, import in `cmd/xk6air/module.go`.

## CLI Usage

```bash
./build/stroppy run <workload> [sql-override] [flags] [-- k6-args]
```

**Positional:**
- 1st: workload — preset-relative path (`tpcc/tx`, `tpcb/procs`, `tpch/tx`), bare preset with a matching `.ts` (`simple`, `tpcds`), `.ts` file, `.sql` file, or inline SQL string
- 2nd (optional): SQL file override (e.g. `tpcc/pico`, `./workloads/tpcc/pico.sql`)

**Driver flags:**
- `-d <preset>` — driver preset: `pg`, `mysql`, `pico`, `ydb`, `noop`
- `-d '{"url":"...","bulkSize":20}'` — raw JSON driver config
- `-D key=value` — override driver field (url, driverType, defaultInsertMethod, defaultTxIsolation, errorMode, bulkSize, pool.*, postgres.*, sql.*, caCertFile, authToken, authUser, authPassword, tlsInsecureSkipVerify); multiple `-D` accumulate
- `-d1 <preset>`, `-D1 key=value` — same for second driver index (multi-driver workloads)

**Script env flags:**
- `-e KEY=VALUE` — set script ENV() value (uppercased); takes precedence over config file and script defaults

**Step control:**
- `--steps step1,step2` — run only listed steps
- `--no-steps step1` — run all steps except listed
- Mutually exclusive

**Config file:**
- Default: `stroppy-config.json` in cwd (auto-loaded if present)
- `-f prod.json` — explicit path
- Precedence (highest→lowest): real env > `-e` > config `env` > `-d/-D` > config `drivers` > script defaults

**k6 passthrough:**
- `-- <k6-args>` after separator, passed directly to k6

**Examples:**
```bash
# TPC-C with postgres
./build/stroppy run tpcc/tx -d pg -D url=postgres://... -- --vus 10 --duration 60s

# TPC-C with picodata, local SQL file (no rebuild needed)
./build/stroppy run ./workloads/tpcc/tx.ts ./workloads/tpcc/pico.sql -d pico -D url=http://...

# TPC-B
./build/stroppy run tpcb/tx -d pg -D url=postgres://... -- --duration 30s

# TPC-H
./build/stroppy run tpch/tx -d pg -D url=postgres://... -e SCALE_FACTOR=0.01

# Noop overhead benchmark
./build/stroppy run simple -d noop -- --vus 4 --duration 10s

# Probe: inspect script ENV declarations and SQL sections
./build/stroppy probe tpcc/tx
```

## Workload Structure

Per-dialect SQL files: `pg.sql`, `mysql.sql`, `pico.sql`, `ydb.sql` under `workloads/{tpcb,tpcc,tpch}/`.

Section layout (must be identical across dialects):
```sql
--+ drop_schema           -- all dialects
--+ create_schema         -- all dialects
--+ create_procedures     -- pg.sql, mysql.sql ONLY
--+ workload_procs        -- pg.sql, mysql.sql ONLY (named query per tx, calls stored proc)
  --= new_order
  --= payment
--+ workload_tx_<txname>  -- all dialects, one per transaction type
  --= step1
  --= step2
```

Two TS variants per workload:
- `procs.ts` — calls stored procs via `workload_procs` section; pg + mysql only; throws at load time on pico/ydb
- `tx.ts` — runs ordered DML steps inside `driver.beginTx()`; SQL drivers (pg/mysql/pico/ydb); has `export default function` and `export const options`

Both `tx.ts` files export a `default` function — `-- --vus N --duration Xs` works for both tpcc and tpcb.

TPC-H has `tpch/tx.ts` only: relational load of 8 tables plus q1–q22 execution; SF=1 answer validation is PostgreSQL-only, while load/query execution has pg/mysql/pico/ydb dialect files.

Relational workloads use `Step("load_data", ...)` and `driver.insertSpec(Rel.table(...))`. `LOAD_WORKERS` controls per-table InsertSpec fan-out where wired:
```bash
./build/stroppy run tpcc/tx -d pg -e LOAD_WORKERS=8 --steps drop_schema,create_schema,load_data
```

**Scenario selection** (`declareScenario(name, defaults)` in `helpers.ts`):
- `DURATION` set → `constant-vus` executor (throughput run)
- `DURATION` unset → `shared-iterations` executor (power run)
- `MAX_DURATION` (default `24h`) lifts k6's hardcoded 10m cap on iteration executors — always pinned
- Tune via env `VUS`/`DURATION`/`ITER`/`MAX_DURATION`, NOT the k6 `-u`/`-d`/`-i` shortflags (see K6 Passthrough footgun)

**SCALE_FACTOR semantics** differ by workload: tpcb and tpcc take an INTEGER (≥1, = branch/warehouse count); tpch and tpcds take a FRACTIONAL row-scale (0.01 ok). tpcds also carries fixed-size static dims (~1.9M rows for `customer_demographics`) that do not shrink with SF.

**Setup vs executor:** k6 emits NO per-iteration builtins (`iteration`, `iteration_duration`) during `setup()` — they fire only inside an executor. A load in `setup()` therefore looks dead (no live metrics), which is why data loads live in `default()` + `GlobalOnce`, not `setup()`.

Isolation by driver in `tx.ts`:
- postgres → `read_committed`
- mysql → `read_committed`
- picodata → `"none"` (**not** `"conn"` — `Begin()` always errors)
- ydb → `serializable`
- Override: `-e TX_ISOLATION=...`

Full isolation type names: `read_uncommitted`, `read_committed`, `repeatable_read`, `serializable`, `db_default`, `conn`, `none`

## TypeScript API

### `helpers.ts`

- `DriverX` — typed driver wrapper with metrics; `DriverX.create().setup()`, `.insertSpec()`, `.exec()`, `.queryRows()`, `.queryRow()`, `.queryValue<T>()`, `.queryCursor()`, `.begin()`, `.beginTx()`
- `TxX` — transaction wrapper; full query API: `exec`, `queryRow`, `queryValue<T>`, `queryRows`, `queryCursor`
- `declareDriverSetup(index, defaults)` — reads CLI driver config, merges over TS defaults; returns `DriverSetup`
- `ENV(name, default?)` — typed env accessor; metadata captured by probe
- `Step(name, fn)` — named execution block with cloud notification
- `InsertMethodName` — `"plain_query" | "plain_bulk" | "native"` (pg→COPY, ydb→BulkUpsert)
- `ErrorModeName` — `"silent" | "log" | "throw" | "fail" | "abort"`
- `DriverTypeName` — `"postgres" | "mysql" | "picodata" | "ydb" | "noop" | "csv"`
- `retry<T>(fn, maxAttempts, isRetryable, onRetry?)`, `retryWithPolicy()`, `txRetryPolicy()` — retry helpers
- `isSerializationError(e)` — detects SQLSTATE 40001 / deadlock for retry decisions
- `once` — run-once guard utility

`TxX` query methods return real values — always use `tx.queryRow()`/`tx.queryValue<T>()` to thread values within a transaction. Synthetic per-VU counters are only justified for PKs with no DB-side value (e.g. synthetic `h_id` on history table for picodata/ydb).

### `datagen.ts`

- `Rel.table(name, opts)` — build an InsertSpec for a table; use `driver.insertSpec(spec)` in `load_data`
- `Attr` — attribute helpers: `rowIndex`, `rowId`, `dictAt`, `dictAtInt/Float`, `lookup`, `blockRef`, `cohortDraw`, `cohortLive`
- `Expr` — literals, `litFloat`, `litNull`, `col`, arithmetic/comparison/logical ops, `if`, stdlib calls
- `Draw` — deterministic load-time distributions: int/float uniform, normal, zipf, NURand, decimal, ascii, phrase, dict, joint, grammar, bernoulli, date
- `DrawRT` — transaction-time random generators; construct at init, call `.next()`/`.sample()`/`.seek()`/`.reset()` in workload code
- `Dict` — inline dictionaries and JSON dictionaries, auto-emitted under InsertSpec dicts when referenced
- `Rel.relationship`, `Rel.lookupPop`, `Rel.cohort`, `Rel.scd2` — relationship, lookup, cohort, and SCD-2 population features
- `InsertMethod` — datagen enum used in `Rel.table({ method })`; driver config `defaultInsertMethod` can pin/override per-spec method

## SQL Syntax Rules

- Query parameters: `:paramName` — converted to `$1, $2...` (PostgreSQL), `?` (MySQL)
- `--+ section_name` — groups statements into sections
- `--= query_name` — names individual queries within a section
- `parse_sql_with_sections()` → section/query lookup function; `parse_sql()` → flat query lookup
- **`--` comment lines inside query bodies are stripped by `parse_sql.ts`** before reaching DB. Use `/* */` block comments inside procedure bodies — except on picodata (see below).

## Picodata-Specific Limits

1. **No `/* */` block comments** at statement head — sbroad parser rejects them. Use `-- ` line comments (stripped by parse_sql before sending).
2. **No `OFFSET` in SELECT** — sbroad doesn't support `LIMIT n OFFSET m`. Branch in ts via `IS_PICODATA`: picodata path uses `queryRows` + `rows[offset]`.
3. **`sql_vdbe_opcode_max` default (45000) too low** for full-scan aggregations. Before tpcc validate_population: `ALTER SYSTEM SET sql_vdbe_opcode_max = 100000000;`
4. **Sharded joins intermittently fail** with `Temporary SQL table TMP_... not found`. Split into two round-trips: fetch key set, then query with inline `IN (...)` list. See `workloads/tpcc/pico.sql` `get_window_items` + `stock_count_in` pattern.

## sqldriver Rows Normalization

`pkg/driver/sqldriver/rows.go` `Values()` converts `[]byte` → `string` for all columns. Normalizes MySQL's CHAR/VARCHAR scan. If adding a new sql.DB-based driver that returns text as non-string, extend this normalization rather than working around it in workloads.

## Go Exploration

```bash
go doc github.com/jackc/pgx/v5.Rows        # pgx Rows interface
go doc github.com/pashagolub/pgxmock/v4 NewPool
go doc ./pkg/driver Rows                    # local interface
```

Prefer `go doc` over grepping source for type/interface definitions. Never read `*.pb.go` — read `.proto` source instead.

## Key Dependencies

- `go.k6.io/k6 v1.8.0` — load testing engine
- `github.com/jackc/pgx/v5` — PostgreSQL driver
- `github.com/grafana/sobek` — JavaScript engine
- `github.com/spf13/cobra` — CLI
- `connectrpc.com/connect` — gRPC
- OpenTelemetry SDKs — metrics export

## K6 Passthrough

- `K6_WEB_DASHBOARD=true` — real-time dashboard
- `K6_WEB_DASHBOARD_EXPORT=report.html` — HTML report
- All k6 CLI flags work after `--` separator
- **Scenarios footgun:** defining `options.scenarios` in the script makes k6 CLI shortflags (`-u`/`-d`/`-i`, `--vus`/`--iterations`/`--duration`) OVERWRITE the entire scenarios block — including `maxDuration`. Passing them after `--` discards the workload's scenario entirely (k6 logs `"cli" level configuration overrode scenarios configuration entirely`). Parameterize scenarios via ENV (`VUS`/`DURATION`/`ITER`/`MAX_DURATION`), never the shortflags.
