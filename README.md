
# Stroppy

[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?logo=discord&logoColor=white)](https://discord.gg/2mSSrkBkHm)
[![Docs](https://img.shields.io/badge/docs-stroppy--io.github.io-blue)](https://stroppy-io.github.io)

Database stress testing CLI tool powered by k6 workload engine.

## Features

- Built-in TPC-B, TPC-C, TPC-DS like workload tests
- Custom test scenarios support via TypeScript
- PostgreSQL, MySQL, Picodata drivers (more DBMSs coming soon)
- Transaction support with configurable isolation levels
- k6-based load generation engine

## Installation

### Pre-built Binaries

Download the latest release from [GitHub Releases](https://github.com/stroppy-io/stroppy/releases).

### Docker

```bash
docker pull ghcr.io/stroppy-io/stroppy:latest
```

```bash
docker build -t stroppy .
```

### Build from Source

Build requirements: Go 1.24.3+

```bash
make install-xk6  # installs k6, used internally by stroppy
make build
```

The binary will be available at `./build/stroppy`.

## Quick Start

Configure the target database via driver flags (defaults to a local PostgreSQL instance):

```bash
stroppy run tpcc/procs -d pg -D url=postgres://user:password@host:5432/dbname
```

### Run Tests

You can run a test from the local directory.

```bash
./stroppy run workloads/simple/simple.ts
```

Many tests are embedded in stroppy. The first argument is a `.ts` workload, the optional second is a `.sql` schema file. Extensions may be omitted.

TPC-B and TPC-C each ship two scripts:
- `procs` — uses stored procedures; supports **PostgreSQL and MySQL**
- `tx` — uses raw transactions; works with **any DB** (PostgreSQL, MySQL, Picodata, YDB)

```bash
stroppy run tpcc/procs        # TPC-C, stored procedures (pg/mysql)
stroppy run tpcc/procs.ts     # same, explicit extension
stroppy run tpcc/tx           # TPC-C, raw transactions (any DB)
stroppy run tpcb/procs        # TPC-B, stored procedures (pg/mysql)
stroppy run tpcb/tx           # TPC-B, raw transactions (any DB)
```

And you can mix builtin tests with your own scripts or SQL files:

```bash
stroppy run tpcb/procs ./my-experimental.sql
stroppy run ./my-tpcb.ts tpcb/pg.sql
```

Use `-d` to select a driver preset and `-D` to override driver options:

```bash
stroppy run tpcc/procs -d pg
stroppy run tpcc/procs -d mysql -D url=mysql://root:pass@localhost:3306/bench
stroppy run tpcc/tx -d pico                   # picodata: use tx variant
stroppy run tpcc/procs -d pg -d1 mysql        # two drivers
```

Pass environment variables to the script with `-e` (keys are auto-uppercased):

```bash
stroppy run tpcc/procs -e pool_size=200
stroppy run tpcc/procs -d pg -e scale_factor=2
```

Use `stroppy help` to explore available topics:

```bash
stroppy help drivers
stroppy help resolution
```

### Probe Tests

Probe inspects a workload and prints its configuration and SQL schema without running it.

```bash
stroppy probe tpcc/procs
stroppy probe tpcc/tx.ts

stroppy help probe
```

### Presets Tree
```
├─ execute_sql
│  └─ execute_sql.ts
├─ simple
│  └─ simple.ts
├─ tests
│  └─ multi_drivers_test.ts sqlapi_test.ts transaction_test.ts
├─ tpcb
│  ├─ procs.ts            (stored procedures — pg/mysql)
│  ├─ tx.ts               (raw transactions  — any DB)
│  └─ pg.sql mysql.sql pico.sql ydb.sql
├─ tpcc
│  ├─ procs.ts            (stored procedures — pg/mysql)
│  ├─ tx.ts               (raw transactions  — any DB)
│  └─ pg.sql mysql.sql pico.sql ydb.sql
└─ tpcds
   ├─ tpcds-scale-(1/10/100/300/1000/3000/10000/30000/50000/100000).sql
   └─ tpcds.ts
```

### Generate Test Workspace

Generate workspace with preset:

```bash
stroppy gen --workdir mytest --preset=simple
```

Check available presets:

```bash
stroppy help gen
```

This creates a new directory with:
- Stroppy binary
- Test configuration files
- TypeScript test templates

Install dependencies:

```bash
cd mytest && npm install
```

## Developing Test Scripts

After generating a workspace:

1. Edit TypeScript test files in your workdir
2. Import stroppy types and use helpers framework.
3. Use k6 APIs for test scenarios
4. Run with `./stroppy run <test-file>.ts`

Look at `simple.ts` and `tpcb/procs.ts` first as a reference.

## Docker Usage

### Using Built-in Workloads

Run directly (--network host to reach localhost databases):

```bash
docker run --network host ghcr.io/stroppy-io/stroppy run simple
```

> Add the tag to image:
> ```bash
> docker tag ghcr.io/stroppy-io/stroppy stroppy
> ```

```bash
docker run --network host stroppy run tpcb/procs \
  -d pg -D url=postgres://user:password@host:5432/dbname
```

Available workloads: `simple`, `tpcb`, `tpcc`, `tpcds`

### Create Persistent Workdir

```bash
# Generate workspace
docker run -v $(pwd):/workspace stroppy gen --workdir mytest --preset=simple
cd mytest

# Run test
docker run -v $(pwd):/workspace stroppy run simple.ts
```

## Advanced Usage

### Using as k6 Extension

Stroppy is built as a k6 extension. If you're familiar with k6, you can use the k6 binary directly to access all [k6 features](https://grafana.com/docs/k6/latest/using-k6/k6-options/reference/):

```bash
# Build both k6 and stroppy binaries
make build

# Use k6 binary directly with all k6 options
./build/k6 run --vus 10 --duration 30s test.ts

# Use k6 output options (JSON, InfluxDB, etc.)
./build/k6 run --out json=results.json test.ts

# All the stroppy commands accessible as extension
./build/k6 x stroppy run workloads/simple/simple.ts
```

The stroppy extensions are available via `k6/x/stroppy` module in your test scripts, giving you full access to both k6 and stroppy capabilities.

## Contribution

### Full Build With Generated Files

Build requirements: Go 1.24.3+, Node.js and npm, git, curl, unzip

```bash
make install-bin-deps
make proto            # build protobuf and ts framework bundle
make build
```

## License

See LICENSE file for details.
