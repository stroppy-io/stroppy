# Stroppy — Agent Context

Database stress testing CLI. A single self-contained Go binary — no k6, no
Node, no TypeScript runtime. Apache 2.0.

## Changelog

`CHANGELOG.md` feeds the docs site — write plain-English one-liners for *users* (not commit-speak), grouped under `## [Unreleased]` by Added/Changed/Fixed. For any user-facing change, add the line **in the same commit** as the change (atomic) — propose it with the diff, don't chase it afterward; for direct/no-PR commits leave it plain text. When the change goes through a PR, after the PR is opened add a follow-up commit that appends the correct link `([#NN](https://github.com/stroppy-io/stroppy/pull/NN))` to that line.

## Binary Layout

`make build` runs `go build -trimpath -o build/stroppy ./cmd/stroppy`. One
binary, one name. There is no longer a separate k6 binary and no
name-dispatch/carriage contract.

## Build & Lint

```
make build          # plain go build — never go build ./... (use the target for -ldflags version injection)
make linter_fix     # run first, auto-fixes formatting — NEVER run casually, it rewrites the whole repo
make linter         # read-only check after linter_fix
make tests          # all tests with race detector and coverage
```

There is no `make proto` and no TypeScript toolchain. The `.pb.go` types under
`pkg/common/proto/stroppy/` are frozen hand-edited Go types (RunConfig,
DriverConfig, etc.), and `pkg/datagen/dgproto` no longer exists — load-time
generation is plain Go under `pkg/gen/` (see `docs/parallelism.md`).

**Embedded FS rebuild rule:** `workloads/` is `//go:embed *` (SQL/JSON/MD only).
If you pass a workload by short name (`tpcc/tx`), the binary serves its SQL from
the embedded snapshot. Edits to `workloads/*.sql` on disk have **no effect** until
`make build` reruns.

**Local path bypass:** If you pass an explicit local `.sql` path
(`./workloads/tpcc/pico.sql`), the runner resolves it from cwd **first** — no
rebuild needed. Use this during the edit-run loop:
```bash
./build/stroppy run tpcc/tx ./workloads/tpcc/pico.sql -d pico -D url=http://...
```

Resolution order for SQL files: **cwd → `~/.stroppy/` → embedded**.

## Directory Map

| Path | Role |
|------|------|
| `cmd/stroppy/` | entrypoint (`main.go` blank-imports the drivers) + cobra subcommands: run, probe, version |
| `cmd/stroppy/commands/run/` | arg parsing, driver/env/step resolution, dispatch to `bench.Run` |
| `pkg/bench/` | Go-native engine: `Workload` interface, `Run`, scenario executor, VU/Bench SDK, metrics sink + summary |
| `internal/workloads/` | the Go workloads (simple, tpcb, tpcc, tpch, tpcds, execute_sql); aggregated by blank import |
| `pkg/driver/dispatcher.go` | driver registry: `RegisterDriver()` + `Dispatch()` |
| `pkg/driver/{postgres,mysql,picodata,ydb,noop,csv}/` | driver implementations |
| `pkg/driver/sqldriver/` | shared sql.DB-backed base (mysql, ydb use this) |
| `pkg/gen/` | imperative generation primitives: Root/Domain/Field scalars, reusable typed Batches, IndexedSource, Permute/SplitMix64 |
| `pkg/datagen/` | row-production seam: `source` (Partitionable/RowSource) + canonical TPC-DS/TPC-H generator adapters (`tpcdsgen`, `tpchgen`) |
| `internal/runner/` | run-config merge, env override parsing, driver presets, config-file load |
| `pkg/common/proto/stroppy/` | frozen Go types (RunConfig, DriverConfig, etc.) — the contract, not codegen |
| `workloads/` | embedded SQL/JSON workloads: tpcb, tpcc, tpch, tpcds |
| `docs/parallelism.md` | InsertRequest parallelism contract and tuning |

## Drivers

| Preset | Type enum | Notes |
|--------|-----------|-------|
| `pg` | DRIVER_TYPE_POSTGRES | pgxpool-based; supports plain_query, plain_bulk, native (COPY) |
| `mysql` | DRIVER_TYPE_MYSQL | sql.DB-backed via sqldriver |
| `pico` | DRIVER_TYPE_PICODATA | sql.DB-backed; `Begin()` always errors — use isolation `"none"` |
| `ydb` | DRIVER_TYPE_YDB | sql.DB-backed; native maps to BulkUpsert |
| `noop` | DRIVER_TYPE_NOOP = 5 | discards all I/O; benchmarks stroppy/framework overhead |
| *(no preset)* | DRIVER_TYPE_CSV = 6 | URL-configured CSV output driver; native-only, no query path |

CSV example:
```bash
./build/stroppy run tpcb/tx -D driverType=csv \
  -D url='/tmp/tpcb-csv?merge=true&workload=tpcb' \
  --steps drop_schema,create_schema,load_data
```

Add driver: package under `pkg/driver/<name>/`, implement `driver.Driver`
(`Insert`, `RunQuery`, `Begin`, `ClassifyError`, `Teardown`), call
`RegisterDriver()` in `init()`, and add a blank import in `cmd/stroppy/main.go`.

Driver `ClassifyError` methods translate backend errors into `driver.ErrorFacts`;
they do not choose workload behavior. Transactional workloads build policy with
`b.TxRetryPolicy(opts)`. Defaults retry serialization, deadlock, lock-timeout,
and unconditional transient facts; unknown facts return errors. Override selected
facts per workload with `TxRetryPolicyOptions.Actions`; set `Idempotent` to allow
conditional transient retries.

## CLI Usage

```bash
./build/stroppy run <workload> [sql-override] [flags]
```

**Positional:**
- 1st: workload — a registered Go workload name (`tpcc/tx`, `tpcb/tx`, `tpch/tx`, `tpcds`, `simple`, `execute_sql`), a `.sql` file, or an inline SQL string (contains spaces)
- 2nd (optional): SQL file override (e.g. `tpcc/pico`, `./workloads/tpcc/pico.sql`)

**Driver flags:**
- `-d <preset>` — driver preset: `pg`, `mysql`, `pico`, `ydb`, `noop`
- `-d '{"url":"...","bulkSize":20}'` — raw JSON driver config
- `-D key=value` — override driver field (url, driverType, defaultTxIsolation, errorMode, bulkSize, pool.*, postgres.*, sql.*, caCertFile, authToken, authUser, authPassword, tlsInsecureSkipVerify); multiple `-D` accumulate
- `-d1 <preset>`, `-D1 key=value` — same for second driver index (multi-driver workloads)

**Env flags:**
- `-e KEY=VALUE` — set workload env value (uppercased); takes precedence over config file and workload defaults

**Step control:**
- `--steps step1,step2` — run only listed steps
- `--no-steps step1` — run all steps except listed
- Mutually exclusive

**Config file:**
- Default: `stroppy-config.json` in cwd (auto-loaded if present)
- `-f prod.json` — explicit path
- Precedence (highest→lowest): real env > `-e` > config `env` > `-d/-D` > config `drivers` > workload defaults

There is **no** `--` k6-args passthrough. Concurrency is env-driven (see Scenario selection).

**Examples:**
```bash
# TPC-C with postgres, 10 VUs for 60s
./build/stroppy run tpcc/tx -d pg -D url=postgres://... -e VUS=10 -e DURATION=60s

# TPC-C with picodata, local SQL file (no rebuild needed)
./build/stroppy run tpcc/tx ./workloads/tpcc/pico.sql -d pico -D url=http://...

# TPC-B fixed-iteration power run
./build/stroppy run tpcb/tx -d pg -D url=postgres://... -e ITER=100

# TPC-H
./build/stroppy run tpch/tx -d pg -D url=postgres://... -e SCALE_FACTOR=0.01

# Noop overhead benchmark
./build/stroppy run simple -d noop -e VUS=4 -e DURATION=10s

# Probe: list embedded presets + driver insert methods
./build/stroppy probe
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

Each Go workload implements the `bench.Workload` interface (`Setup`, `Iterate`,
`Teardown`) in `internal/workloads/<name>/`. TPC-B and TPC-C each ship two
registered variants:
- `procs` — calls stored procs via the `workload_procs` section; pg + mysql only
- `tx` — runs ordered DML steps inside `driver.beginTx()`; all SQL drivers (pg/mysql/pico/ydb)

TPC-H (`tpch/tx`) does a relational load of 8 tables plus q1–q22 execution;
SF=1 answer validation is PostgreSQL-only, while load/query execution has
pg/mysql/pico/ydb dialect files. TPC-DS (`tpcds`) loads all 24 tables and runs
the 99 query suite.

Relational loads use `b.Step("load_data", ...)` and `b.Insert(ctx, req)` with
a `driver.InsertRequest`. `LOAD_WORKERS` controls the per-request worker
fan-out where wired:
```bash
./build/stroppy run tpcc/tx -d pg -e LOAD_WORKERS=8 --steps drop_schema,create_schema,load_data
```

**Scenario selection** (`readScenario` in `pkg/bench/runtime.go`, env-driven):
- `DURATION` set → `constant-vus` executor (throughput run)
- `DURATION` unset → `shared-iterations` executor (power run)
- Tune via env `VUS` / `DURATION` / `ITER`. There are no k6 shortflags.

**SCALE_FACTOR semantics** differ by workload: tpcb and tpcc take an INTEGER (≥1, = branch/warehouse count); tpch and tpcds take a FRACTIONAL row-scale (0.01 ok). tpcds also carries fixed-size static dims (~1.9M rows for `customer_demographics`) that do not shrink with SF.

**Setup vs executor:** the data load lives in the workload body guarded by
`GlobalOnce` (a once-per-process barrier), not in a separate setup phase with
no live metrics. Load progress emits metrics as it runs.

Isolation by driver in the `tx` variants:
- postgres → `read_committed`
- mysql → `read_committed`
- picodata → `"none"` (**not** `"conn"` — `Begin()` always errors)
- ydb → `serializable`
- Override: `-e TX_ISOLATION=...`

Full isolation type names: `read_uncommitted`, `read_committed`, `repeatable_read`, `serializable`, `db_default`, `conn`, `none`

## SQL Syntax Rules

- Query parameters: `:paramName` — converted to `$1, $2...` (PostgreSQL), `?` (MySQL)
- `--+ section_name` — groups statements into sections
- `--= query_name` — names individual queries within a section
- The SQL parser (`pkg/bench` SQL loading) resolves sections/queries by name
- **`--` comment lines inside query bodies are stripped before reaching DB.** Use `/* */` block comments inside procedure bodies — except on picodata (see below).

## Picodata-Specific Limits

1. **No `/* */` block comments** at statement head — sbroad parser rejects them. Use `-- ` line comments (stripped before sending).
2. **No `OFFSET` in SELECT** — sbroad doesn't support `LIMIT n OFFSET m`. Branch in the workload: fetch the key set with `queryRows`, then index `rows[offset]`.
3. **`sql_vdbe_opcode_max` default (45000) too low** for full-scan aggregations. Before tpcc validate_population: `ALTER SYSTEM SET sql_vdbe_opcode_max = 100000000;`
4. **Sharded joins intermittently fail** with `Temporary SQL table TMP_... not found`. Split into two round-trips: fetch key set, then query with inline `IN (...)` list. See `workloads/tpcc/pico.sql` `get_window_items` + `stock_count_in` pattern.

## sqldriver Rows Normalization

`pkg/driver/sqldriver/rows.go` `Values()` converts `[]byte` → `string` for all columns. Normalizes MySQL's CHAR/VARCHAR scan. If adding a new sql.DB-based driver that returns text as non-string, extend this normalization rather than working around it in workloads.

## Go Exploration

```bash
go doc github.com/jackc/pgx/v5.Rows        # pgx Rows interface
go doc ./pkg/driver Rows                    # local interface
go doc ./pkg/bench Workload                 # workload interface
```

Prefer `go doc` over grepping source for type/interface definitions. The `.pb.go`
files under `pkg/common/proto/stroppy/` are frozen hand-edited types — read them
directly (there is no `.proto` source).

## Key Dependencies

- `github.com/jackc/pgx/v5` — PostgreSQL driver
- `github.com/spf13/cobra` — CLI
- `google.golang.org/grpc` — YDB SDK transport
- `google.golang.org/protobuf` — frozen `.pb.go` types compile against it
