
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

### Build from Source

Build requirements:
- Go 1.24.3+

```
make build-all-linux-x64
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
