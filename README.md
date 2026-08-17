
# Stroppy

[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?logo=discord&logoColor=white)](https://discord.gg/2mSSrkBkHm)
[![Docs](https://img.shields.io/badge/docs-stroppy--io.github.io-blue)](https://stroppy-io.github.io)

Database stress testing CLI. A single self-contained Go binary — no k6, no
Node, no runtime dependencies.

## Features

- Built-in TPC-B, TPC-C, TPC-H, and TPC-DS workload tests
- Deterministic relational data generation with `driver.insertSpec`
- PostgreSQL, MySQL, YDB, Picodata, CSV, and noop drivers
- Transaction support with configurable isolation levels
- Go-native load generation and metrics

## Installation

### Pre-built Binaries

Download the latest release from [GitHub Releases](https://github.com/stroppy-io/stroppy/releases).

### Docker

```bash
docker pull ghcr.io/stroppy-io/stroppy:latest
# or build locally
docker build -t stroppy .
```

### Build from Source

Build requirements: Go 1.24.3+

```bash
make build
```

The binary will be available at `./build/stroppy`.

## Quick Start

Configure the target database via driver flags:

```bash
stroppy run tpcc/tx -d pg -D url=postgres://user:password@host:5432/dbname
```

The first argument selects a built-in workload; the optional second is a `.sql`
schema file override. Inline SQL is also accepted:

```bash
stroppy run tpcc/tx           # TPC-C, raw transactions (any DB)
stroppy run tpcc/procs        # TPC-C, stored procedures (pg/mysql)
stroppy run tpcb/tx           # TPC-B, raw transactions (any DB)
stroppy run tpch/tx           # TPC-H, relational load + query suite
stroppy run tpcds --scale-factor 1   # TPC-DS, load 24 tables + 99 queries
stroppy run queries.sql       # execute a SQL file
stroppy run "select 1"        # execute inline SQL
```

TPC-B and TPC-C each ship two variants:
- `procs` — uses stored procedures; supports **PostgreSQL and MySQL**
- `tx` — uses raw transactions; works with **all SQL drivers** (PostgreSQL, MySQL, Picodata, YDB)

TPC-H loads all eight tables and runs the 22 query suite. TPC-DS generates and
loads all 24 tables (faithful `dsdgen` port) and runs the 99 query suite; scale
either via `--scale-factor`.

Use `-d` to select a driver preset and `-D` to override driver options:

```bash
stroppy run tpcc/tx -d pg
stroppy run tpcc/tx -d mysql -D url=mysql://root:pass@localhost:3306/bench
stroppy run tpcc/tx -d pico
stroppy run tpcc/tx -d pg -d1 mysql        # two drivers
stroppy run simple -d noop                  # framework/runner overhead only
```

Registered workloads expose typed direct flags. Select the executor explicitly;
use `stroppy run <workload> --help` to inspect that workload's parameters:

```bash
stroppy run tpcc/tx --executor constant-vus --vus 10 --duration 60s
stroppy run tpcc/tx --executor shared-iterations --iterations 100
stroppy run tpcc/tx --scale-factor 10 --load-workers 8
stroppy run tpcc/tx -e pool_size=200   # legacy env compatibility
```

Collect repeated settings in `stroppy-config.json` or an explicit `-f` file.
Typed scenario parameters belong under `run`, workload parameters under `params`,
and the string-valued `env` map remains available for compatibility:

```json
{
  "script": "tpcc/tx",
  "run": {"executor": "constant-vus", "vus": 10, "duration": "60s"},
  "params": {"scaleFactor": 10, "loadWorkers": 8}
}
```

```bash
stroppy run -f prod.json
```

Typed parameter precedence is: CLI flag > process environment > `-e` > matching
`run`/`params` config > config `env` > declared default. Driver precedence is
`-d/-D` > config `drivers`. Legacy `DURATION` inference remains compatible but
warns; use an explicit executor.

Use `stroppy help` to explore available topics:

```bash
stroppy help drivers
stroppy help config-file
```

### Probe

Probe lists registered workload parameter schemas, embedded presets, and the insert
methods each driver supports:

```bash
stroppy probe
stroppy probe -o json
```

### Workload Tree

```
├─ execute_sql            (run a .sql file or inline SQL)
├─ simple                 (smoke / overhead benchmark)
├─ tpcb
│  ├─ tx                  (raw transactions — any DB)
│  └─ pg.sql mysql.sql pico.sql ydb.sql
├─ tpcc
│  ├─ tx                  (raw transactions — any DB)
│  ├─ procs               (stored procedures — pg/mysql)
│  └─ pg.sql mysql.sql pico.sql ydb.sql ydb_no_indexes.sql
├─ tpch
│  ├─ tx                  (relational load + 22 queries)
│  └─ pg.sql mysql.sql pico.sql ydb.sql distributions.json answers_sf1.json
└─ tpcds
   ├─ tpcds               (load 24 tables + 99 queries)
   └─ schema.pg.sql schema.mysql.sql schema.pico.sql schema.ydb.sql \
        pg.sql mysql.sql pico.sql ydb.sql answers_sf1.json
```

## Docker Usage

Run directly (`--network host` to reach localhost databases):

```bash
docker run --network host ghcr.io/stroppy-io/stroppy run simple
docker run --network host stroppy run tpcc/tx \
  -d pg -D url=postgres://user:password@host:5432/dbname
```

Available workloads: `simple`, `tpcb/tx`, `tpcc/tx`, `tpch/tx`, `tpcds`, `execute_sql`.

## License

See LICENSE file for details.
