# Dependency refresh: short performance comparison

Date: 2026-11-12. Base: PR [#277](https://github.com/stroppy-io/stroppy/pull/277), commit `a802a3911392111bd23bc3f7f5a81f468671801e`. Updated source: `84076ac62c0a19d402a6aa63d3bcc69fc2677f00` on `chore/release-dependencies`.

## Result

The dependency refresh is functionally validated, but it is **not performance-neutral**. OpenTelemetry 1.47.0 reduces the noop framework transaction ceiling by about one third. Keep this visible when deciding whether to release the complete refresh or defer the telemetry upgrade; no dependency was silently rolled back.

Three alternating pairs, both binaries built with Go 1.27.4, median transactions/s:

| Tier | VUs | PR #277 | Updated | Change |
| --- | ---: | ---: | ---: | ---: |
| noop | 1 | 826,795 | 542,829 | -34.35% |
| noop | 8 | 4,658,982 | 2,976,972 | -36.10% |
| pg-wire | 1 | 9,609 | 9,435 | -1.82% |
| pg-wire | 8 | 37,609 | 37,237 | -0.99% |

All transaction phases recorded zero failed iterations. Load medians were 15.09M versus 14.45M rows/s for noop, and 10.91M versus 10.57M rows/s for wire. These 17–24 ms load phases are too short and variable to support a load-regression claim. The small wire differences are descriptive, not a statistical significance claim.

## Method

- Apple M5 Pro, Mac17,8, 18 logical CPUs, 48 GiB RAM, macOS arm64.
- `GOMAXPROCS=8`, eight VUs for parallel phases, 250,000 load rows, three seconds per transaction phase.
- Same cached pg-noop v0.2.0 on loopback; no real database in these measurements.
- `make build` for both binaries. The unchanged PR source was extracted with `git archive` and rebuilt with `GOTOOLCHAIN=go1.27.4`; the final updated binary was also built with Go 1.27.4.
- No other task-owned build or test was running during the alternating pairs. PostgreSQL/MySQL test containers had been stopped.
- `baseline --no-save` avoids modifying the user's baseline history.

```sh
GOMAXPROCS=8 ./build/stroppy-before-go1274 baseline \
  --duration 3s --vus 8 --rows 250000 --download never --json --no-save
GOMAXPROCS=8 ./build/stroppy baseline \
  --duration 3s --vus 8 --rows 250000 --download never --json --no-save
```

Raw results: `paired-before-{1,2,3}.json` and `paired-after-{1,2,3}.json`.

The first five before/after runs (`before-*.json`, `after-*.json`) used Go 1.27.3 for both versions and reproduced the same pattern: noop -34.3% single-VU and -37.7% parallel, wire about -1.6% and -1.8%. Those runs preceded the Go security patch and later compatibility changes; use the alternating Go 1.27.4 results above for the final comparison.

## Telemetry isolation

An alternate module file retained every updated non-telemetry dependency and replaced only the seven OTel core/API/SDK/exporter modules with 1.44.0. This is diagnostic only; the committed `go.mod` keeps 1.47.0.

The control's noop baseline (`otel-control.json`) recovered 796,824 single-VU and 4,317,614 parallel transactions/s. Existing metric microbenchmarks independently isolate recording overhead. Six samples per case, Go 1.27.4, `GOMAXPROCS=8`, 300 ms benchmark windows:

- `BenchmarkTrendRecord`: 39.34 ns/op with OTel 1.44.0, 71.15 ns/op with 1.47.0; +80.84%, p=0.002.
- `BenchmarkMetricAlternatingSeries/parallel=false`: 50.18 versus 95.86 ns/op; +91.06%, p=0.002.
- `BenchmarkMetricAlternatingSeries/parallel=true`: 47.65 versus 51.39 ns/op; +7.84%, p=0.002.
- All these cases remain zero bytes and zero allocations per operation.

See `metrics-otel-control-go1274.txt`, `metrics-updated-go1274.txt`, and `metrics-comparison.txt`. Earlier three-sample metric runs are retained as exploratory evidence only.

```sh
cp go.mod control.mod
cp go.sum control.sum
go mod edit -modfile=control.mod \
  -replace=go.opentelemetry.io/otel=go.opentelemetry.io/otel@v1.44.0 \
  -replace=go.opentelemetry.io/otel/metric=go.opentelemetry.io/otel/metric@v1.44.0 \
  -replace=go.opentelemetry.io/otel/trace=go.opentelemetry.io/otel/trace@v1.44.0 \
  -replace=go.opentelemetry.io/otel/sdk=go.opentelemetry.io/otel/sdk@v1.44.0 \
  -replace=go.opentelemetry.io/otel/sdk/metric=go.opentelemetry.io/otel/sdk/metric@v1.44.0 \
  -replace=go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc=go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc@v1.44.0 \
  -replace=go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp=go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp@v1.44.0
GOMAXPROCS=8 go test -mod=mod -modfile=control.mod ./pkg/bench \
  -run '^$' -bench 'BenchmarkTrendRecord|BenchmarkMetricAlternatingSeries' \
  -benchmem -benchtime=300ms -count=6
GOMAXPROCS=8 go test ./pkg/bench \
  -run '^$' -bench 'BenchmarkTrendRecord|BenchmarkMetricAlternatingSeries' \
  -benchmem -benchtime=300ms -count=6
```

## Dependency coverage and exceptions

Updated 39 explicit Go module selections, including PostgreSQL/MySQL/YDB drivers, decimal, gRPC/Protobuf, telemetry, standard Go support modules, and test dependencies. Current same-major versions were checked for modules actually imported by normal, integration, and optional-integration test graphs on Darwin/arm64, Linux/amd64, and Windows/amd64; no available updates remained in those imported module sets. Historical modules selected only by upstream module metadata are not artificially promoted to direct dependencies.

- Test-only JWT moved from v4 to stable v5.3.1. Yandex's SDK still transitively uses JWT v4.5.2.
- Test-only pgxmock moved to v6.0.0-beta2: stable v4/v5 do not implement the `Rows.TypeMap` and `BatchResults.Tx` interfaces needed by pgx 5.13.0. No prerelease runtime library was introduced.
- YDB native values use the SDK's current `types` package instead of deprecated `table/types`.
- golangci-lint 2.15.0, setup-go v7, cleanup checkout v7, Alpine runtime 3.24, UI kit 0.7.21, and integration collector 0.165.0 are updated. Other action major tags already resolve to their current releases.
- Go now requires 1.27.4, including generated project metadata and the private compiler's checksum-pinned archives. Go 1.27.3 was flagged for GO-2026-6859; the final scan reports no vulnerabilities. Archive hashes come from https://go.dev/dl/.
- pg-noop remains v0.2.0. v0.3.0 provides Linux x86-64 only, while Stroppy embeds/downloads Linux and Darwin on amd64 and arm64. Updating it would remove three supported baseline targets.
- Macavity's audited commit is already the fork's latest commit. TiUP 1.17.1 is current.
- PostgreSQL 17, MySQL 8.0, TiDB 8.5.8, and Picodata 26.2.1 remain compatibility-test targets, not bundled application libraries. Floating optional test images remain floating. No database-major support policy was changed.
- uPlot remains 1.6.32, the version shipped in the latest upstream UI kit. Fonts, mark, and font licences are unchanged.

## Validation

- `make build` passed; final binary uses Go 1.27.4, pgx 5.13.0, and OTel 1.47.0.
- `make linter_fix` followed by read-only `make linter` passed without disabling checks.
- `make tests` passed with race detection and coverage: 60 packages reported successful test runs.
- `make modulecompat` passed the complete external-consumer, published-project, starter round-trip, and documented-sample matrix.
- `make integration` passed PostgreSQL, MySQL, CSV, and OTLP checks; 172.343 seconds. Test services were stopped afterward.
- `go mod verify` passed. `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` reported `No vulnerabilities found.`
- Six chart adapter tests and report package tests passed. A real noop CLI run exported captured metrics and standalone HTML, rendered and inspected in Chromium with working interactive charts and static toggles.
- `docker build --pull` passed using the updated compiler/runtime images. Container `version --json` and `baseline --quick --download never` passed; embedded noop and wire tiers had no failed iterations. The image's version is `unknown` because this local Docker smoke did not pass a release `VERSION` build argument.
- Optional database suites and heavy SF=1 answer suites were not run. No PR, tag, push, or release was created.

Compatibility failures found and fixed: pgx now treats `+` literally in connection URIs, its detached background connection attempts need explicit pool-lifetime cancellation at dial/hooks, current collector adds scope labels to every series, YDB moved its native type API, and updated lint/format tools disagree on direct blank error-channel receives. Exact collector sample assertions remain strict.
