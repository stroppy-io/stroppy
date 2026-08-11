package help

func init() {
	Register(Topic{
		Name:  "datagen",
		Short: "Relational data generation: the InsertSpec load path",
		Long: `DATAGEN

  Stroppy's load path is the relational data-generation framework. Workloads
  build an InsertSpec (a protobuf message describing a table's shape and
  size), hand it to the driver, and rows are streamed through a pure-function
  generator implemented in the Go runtime under pkg/datagen/.

  There is no TypeScript surface and no k6: the generator, the spec, and the
  driver are all Go. Workload authors write Go against the bench SDK.

LOAD FLOW

  1. The workload builds an InsertSpec (table, columns, seed, generator
     source, optional parallelism) — see pkg/datagen/dgproto for the wire
     type and pkg/bench/query.go for InsertSpec helpers.

  2. The workload calls b.InsertSpec(ctx, spec) inside a load_data step.
     The bench engine forwards the spec to the driver and emits metrics
     (insert_duration, insert_error_rate) plus optional progress reports.

  3. Each driver implements driver.InsertSpec:
       postgres  -> COPY (native) or bulk INSERT
       mysql     -> bulk INSERT via sqldriver
       picodata  -> sql.DB bulk path
       ydb       -> BulkUpsert (native)
       csv       -> write rows to CSV files
       noop      -> discard (benchmarks framework overhead)

DETERMINISM AND PARALLEL LOAD

  Each generated value is a pure function of the table seed, attribute path,
  and row index. That makes generated rows reproducible and lets drivers
  split a table into independent worker ranges — any worker can start at any
  row with no warm-up.

  Workloads that support parallel load read LOAD_WORKERS and pass it into
  the InsertSpec fan-out:

    stroppy run tpcc/tx -d pg -e LOAD_WORKERS=8 \
      --steps drop_schema,create_schema,load_data

CSV OUTPUT

  CSV is useful for reference datasets. It supports relational InsertSpec
  loads only (no runtime query path):

    stroppy run tpcb/tx -D driverType=csv \
      -D url='/tmp/tpcb-csv?merge=true&workload=tpcb' \
      --steps drop_schema,create_schema,load_data

REFERENCES

  docs/datagen-framework.md   Workload-author guide (note: some sections
                              still describe the removed TS surface; the
                              Go runtime under pkg/datagen/ is authoritative)
  docs/parallelism.md         Parallel InsertSpec contract and tuning
  internal/workloads/simple/  Minimal Go workload
  internal/workloads/tpcb/    Small relational workload
  internal/workloads/tpch/    Relationship and dictionary-heavy workload
  pkg/datagen/                Go generator runtime
  pkg/bench/query.go          InsertSpec helpers (InsertSpec, TpchLoad, ...)
`,
	})
}
