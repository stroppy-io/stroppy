package help

func init() {
	Register(Topic{
		Name:  "datagen",
		Short: "Deterministic data generation and the typed load path",
		Long: `DATAGEN

  Stroppy's load path is deterministic Go data generation. A workload builds a
  schema (gen.SchemaBuilder), fills reusable typed batches from per-field
  primitives (pkg/gen), and streams them to the driver through driver.Insert.
  There is no TypeScript surface, no k6, and no relational expression AST: the
  generator, the batch, and the driver are all ordinary Go.

LOAD FLOW

  1. The workload describes the row shape with gen.SchemaBuilder and binds
     typed column handles from it.

  2. Inside a load_data step the workload drives a gen.IndexedSource (or a
     stateful TPC generator) over entities and hands the driver a
     driver.InsertRequest{Table, Method, Workers, Source}.

  3. The workload calls b.Insert(ctx, req). The bench engine forwards the
     request to the driver and emits metrics (insert_duration,
     insert_error_rate) plus optional progress reports.

  4. Each driver consumes the typed batch stream:
       postgres  -> COPY (native) or bulk INSERT
       mysql     -> bulk INSERT via sqldriver
       picodata  -> sql.DB bulk path
       ydb       -> BulkUpsert (native)
       csv       -> write rows to CSV files
       noop      -> discard (benchmarks framework overhead)

DETERMINISM AND PARALLEL LOAD

  pkg/gen IndexedSource values are pure functions of the run seed, versioned
  field name, and entity index. Canonical TPC-H/TPC-DS adapters preserve their
  generator seeds and support deterministic range seeking. Both source types
  reproduce rows across workers, batch sizes, and partition boundaries, so a
  driver can split a table into worker ranges with no warm-up.

  Workloads that support parallel load read LOAD_WORKERS and pass it as the
  request worker count:

    stroppy run tpcc/tx -d pg -e LOAD_WORKERS=8 \
      --steps drop_schema,create_schema,load_data

CSV OUTPUT

  CSV is useful for reference datasets. It is load-only (no query path):

    stroppy run tpcb/tx -D driverType=csv \
      -D url='/tmp/tpcb-csv?merge=true&workload=tpcb' \
      --steps drop_schema,create_schema,load_data

REFERENCES

  docs/parallelism.md         InsertRequest parallelism contract and tuning
  internal/workloads/simple/  Minimal Go workload
  internal/workloads/tpcb/    Small relational workload
  internal/workloads/tpch/    Stateful canonical generator
  pkg/gen/                    Deterministic generation primitives
  pkg/bench/query.go          b.Insert entrypoint
`,
	})
}
