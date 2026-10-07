# Public workload examples

These packages use only the supported authoring API: `pkg/bench`, `pkg/gen`,
`pkg/report`, and the root application shell. They require Go 1.27 or newer.
To use them outside this repository, copy `query.go`, `accounts.go`, and
`mirror.go` into your own importable package, change the package name as needed,
and require `github.com/stroppy-io/stroppy/v6`. No internal imports or built-in
workload dependencies are needed. The external-module test does exactly this
against a release-shaped module archive, without a `replace` directive.

Run the example catalog from the repository:

```bash
go run ./examples/authoring/main probe -o json
go run ./examples/authoring/main run example/query -d noop --iterations 10
go run ./examples/authoring/main run example/accounts -d noop --iterations 10
go run ./examples/authoring/main run example/mirror \
  -dprimary noop -dsecondary noop --iterations 10 --report-format json
```

- `Query` declares standard run parameters and executes a parameterized query.
- `Accounts` loads ordinary struct rows through `gen.FromRows`, runs managed
  transfers with whole-body retry, verifies owned generic reads, and contributes
  a final report from typed metrics and the actual measurement window.
- `Mirror` uses named driver references, a step-selected default database, a
  second database facade, finite metric labels, and a directly encoded payload.
  The two queries are independent, not an atomic distributed transaction.

**Accounts creates and drops `stroppy_example_accounts`. Use a disposable database
where that table does not already exist.** Noop exercises generation, insertion,
and transaction bookkeeping, but does not verify database contents. PostgreSQL
and MySQL runs additionally read rows and check that transfers preserve the total
balance:

```bash
go run ./examples/authoring/main run example/accounts -d pg \
  -D url='postgres://user:password@localhost:5432/scratch' \
  --rows 100 --load-workers 2 --iterations 20 --report-format json
```

A single-test entrypoint can be smaller:

```go
func main() { stroppy.Main(authoring.Accounts) }
```

For the installed catalog, export a `Test` value from your package and register
it in `init` with `bench.Register(Test)`. Then `stroppy build ./workload` links it
into the managed runtime. Explicit `bench.NewCatalog` does not require global
registration.

See [the authoring guide](../../docs/workload-authoring-api.md) for input
precedence, filters, cancellation/drain semantics, and the v6 compatibility
boundary.
