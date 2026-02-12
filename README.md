
# Stroppy

Database stress testing CLI tool powered by k6 workload engine.

## Features

- Built-in TPC-B, TPC-C, TPC-DS like workload tests
- Custom test scenarios support via TypeScript
- PostgreSQL driver (more databases coming soon)
- k6-based load generation engine

## Installation

### Pre-built Binaries

Download the latest release from [GitHub Releases](https://github.com/stroppy-io/stroppy/releases).

### Docker

Pull the pre-built Docker image:

```bash
docker pull stroppy-io/stroppy:latest
```

Or build from source:

```bash
docker build -t stroppy .
```

### Build from Source

Build requirements:
- Go 1.24.3+

```
make build
```

The binary will be available at `./build/stroppy`.

## Quick Start

### Generate Test Workspace

```
# Generate workspace with preset

stroppy gen --workdir mytest --preset=simple

# Or for execute_sql preset

stroppy gen --workdir mytest --preset=execute_sql

# Check available presets

stroppy help gen
```

This creates a new directory with:
- Stroppy binary
- Test configuration files
- TypeScript test templates

> You can also run test scripts without workdir or any preparations.

### Install Dependencies

```
cd mytest
npm install
```

### Run Tests

```
# Run simple test

./stroppy run simple.ts

# Run SQL execution test

./stroppy run execute_sql.ts tpcb_mini.sql
```

## Developing Test Scripts

After generating a workspace:

1. Edit TypeScript test files in your workdir
2. Import stroppy protobuf types from generated `stroppy.pb.ts` and use helpers framework.
3. Use k6 APIs for test scenarios
4. Run with `./stroppy run <test-file>.ts`

Look at  `simple.ts` and `tpcds.ts` first as a reference.

## Docker Usage

### Quick Start

```bash
# Generate workspace
docker run -v $(pwd):/workspace stroppy gen --workdir mytest --preset=simple
cd mytest

# Run test
docker run -v $(pwd):/workspace stroppy run simple.ts
```

### Using Built-in Workloads

```bash
docker run -e DRIVER_URL="postgres://user:password@host:5432/dbname" \
  stroppy run /workloads/tpcb/tpcb.ts /workloads/tpcb/tpcb.sql
```

Available workloads: `simple`, `tpcb`, `tpcc`, `tpcds`

## Advanced Usage

### Using as k6 Extension

Stroppy is built as a k6 extension. If you're familiar with k6, you can use the k6 binary directly to access all k6 features:

```bash
# Build both k6 and stroppy binaries
make build

# Access k6 CLI features
./build/k6 --help

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

Build requirements:
- Go 1.24.3+
- Node.js and npm
- git, curl, unzip

```
# Install binary dependencies

make install-bin-deps

# Build protocol buffers and ts framework bundle

make proto

# Build k6 with stroppy extensions

make build-k6

# Build stroppy binary

make build
```

## License

See LICENSE file for details.
