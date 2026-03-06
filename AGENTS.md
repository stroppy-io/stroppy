# Stroppy - Project Context

Database stress testing CLI tool powered by k6 workload engine. Apache 2.0 licensed.

## Architecture Overview

Stroppy is a **k6 extension** (`k6/x/stroppy`) that adds database-specific capabilities to k6's load testing engine. Test scripts are written in TypeScript, transpiled via esbuild, and executed inside k6's Sobek JavaScript runtime.

### Binary Layout

- `stroppy` CLI wraps k6 with convenience commands (`gen`, `run`, `version`)
- `k6` binary is also built with the stroppy extension embedded
- Users can use either `stroppy run <script.ts>` or `./build/k6 run <script.ts>`

### Core Components

| Component | Path | Purpose |
|-----------|------|---------|
| CLI commands | `cmd/stroppy/commands/` | `gen`, `run`, `version` subcommands via cobra |
| k6 module | `cmd/xk6air/` | Registers `k6/x/stroppy` module, manages per-VU driver/generator instances |
| Driver interface | `pkg/driver/dispatcher.go` | Registry pattern: `RegisterDriver()` + `Dispatch()` |
| PostgreSQL driver | `pkg/driver/postgres/` | pgxpool-based, supports PLAIN_QUERY and COPY_FROM insertion |
| Data generators | `pkg/common/generate/` | Uniform, Normal, Zipfian distributions; int/float/string/uuid/bool/datetime/decimal |
| TypeScript framework | `internal/static/` | `helpers.ts` (R/S/AB/DriverX), `parse_sql.ts`, generated type bindings |
| Script runner | `internal/runner/` | esbuild transpilation, config extraction via Sobek, k6 process management |
| Schema definitions | `proto/stroppy/` | config, descriptor, common, runtime, cloud schemas |
| Built-in workloads | `workloads/` | simple, tpcb, tpcc, tpcds presets |

### Driver System

Drivers register themselves via `init()` using `driver.RegisterDriver()`. The dispatcher looks up the constructor by `DriverConfig_DriverType` enum. To add a new driver:

1. Create package under `pkg/driver/<name>/`
2. Implement `driver.Driver` interface (InsertValues, RunQuery, Teardown, Configure)
3. Call `driver.RegisterDriver()` in `init()`
4. Import the package in `cmd/xk6air/module.go` for side-effect registration

### TypeScript API (helpers.ts)

- `C` - Const generators: `C.str()`, `C.int32()`, `C.int64()`, `C.uint32()`, `C.uint64()`, `C.float()`, `C.double()`, `C.decimal()`, `C.datetime()`, `C.bool()`, `C.uuid()`
- `R` - Random/range generators: `R.str()`, `R.int32()`, `R.int64()`, `R.uint32()`, `R.uint64()`, `R.float()`, `R.double()`, `R.decimal()`, `R.datetime()`, `R.bool()`, `R.uuid()`, `R.uuidSeeded()`, `R.group()`, `R.groups()`
- `S` - Sequence (unique) generators: `S.str()`, `S.int32()`, `S.int64()`, `S.uint32()`, `S.uint64()`, `S.uuid()`
- `AB` - Alphabets: `en`, `enNum`, `num`, `enUpper`, `enSpc`, `enNumSpc`
- `Dist` - Distribution helpers: `Dist.normal()`, `Dist.uniform()`, `Dist.zipf()`
- `setSeed()` - Set module-wide default seed (0 = random, >0 = fixed)
- `Rule` / `GroupRule` - Generation rules with `.gen(seed?)` method to create generators
- `DriverX` - Typed driver wrapper with metrics tracking; `DriverX.fromConfig()`, `.insert()`, `.runQuery()`
- `InsertMethodName` - `"plain_query" | "copy_from"` — friendly string type for `DriverX.insert()` method option
- `Step()` - Named execution blocks with cloud notification; also `Step.begin()` / `Step.end()`

### SQL Syntax

- Query parameters use `:paramName` syntax, converted to PostgreSQL `$1, $2...` placeholders
- SQL files support structured parsing:
  - `--+ section_name` groups SQL statements into sections
  - `--= query_name` names individual queries within sections
- `parse_sql_with_groups()` returns `Record<string, ParsedQuery[]>`

### UUID Generator Variants

Four variants available via `Generation.Rule.kind` in the proto:

| Proto field | Behavior |
|---|---|
| `uuid_random = true` | Truly random v4 UUID; seed ignored |
| `uuid_seeded = true` | Deterministic v4 UUID sequence; same seed → same sequence (ChaCha8 PRNG) |
| `uuid_seq { max: "..." }` | Sequential counter encoded as UUID; `min` defaults to nil UUID (`00000...0`) |
| `uuid_const { value: "..." }` | Fixed UUID repeated on every call |

Generator factory entry point: `NewValueGeneratorByRule` in `pkg/common/generate/value.go`.

### Build System

- `make build` - Builds k6 with xk6air extension via xk6
- `make proto` - Generates Go, TypeScript, gRPC, docs from proto files; **wipes `pkg/common/proto/*` before regenerating** — never hand-edit generated files
- `make install-bin-deps` - Installs protoc plugins, xk6, esbuild, etc.
- Go 1.24.3+, Node.js required for full build

### Key Dependencies

- go.k6.io/k6 v1.6.0 (load testing engine)
- github.com/jackc/pgx/v5 (PostgreSQL driver)
- github.com/grafana/sobek (JavaScript engine for config extraction)
- github.com/spf13/cobra (CLI framework)
- connectrpc.com/connect (gRPC)
- OpenTelemetry SDKs (metrics export)

### K6 Integration

- k6 web dashboard: `K6_WEB_DASHBOARD=true` enables real-time dashboard
- HTML report export: `K6_WEB_DASHBOARD_EXPORT=report.html`
- All k6 CLI flags pass through after `--` separator: `stroppy run script.ts -- --vus 10 --duration 30s`
- k6 scenarios, thresholds, and metrics all work natively

### Docker

- Image: `ghcr.io/stroppy-io/stroppy:latest`
- Built-in workloads available at `/workloads/` inside container
- `DRIVER_URL` env var for database connection
- `--network host` for localhost database access

### Documentation Site

Docusaurus-based docs live in the GitHub Pages site at `stroppy-io.github.io`.
