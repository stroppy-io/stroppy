package help

func init() {
	Register(Topic{
		Name:  "datagen",
		Short: "Relational data generation with Rel.table and InsertSpec",
		Long: `DATAGEN

  Stroppy's current load path is the relational data-generation framework.
  Workloads declare table shapes in TypeScript, serialize them as InsertSpec
  protobuf messages, and stream rows through the selected driver.

CORE API

  Import datagen builders from datagen.ts:

    import { Attr, Draw, DrawRT, Expr, InsertMethod, Rel } from "./datagen.ts";

  A table load is declared with Rel.table:

    const accounts = Rel.table("accounts", {
      size: 100_000,
      seed: 0xA11CE,
      method: InsertMethod.NATIVE,
      parallelism: LOAD_WORKERS || undefined,
      attrs: {
        aid: Attr.rowId(),
        bid: Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(100_000)), Expr.lit(1)),
        abalance: Draw.intUniform({ min: Expr.lit(0), max: Expr.lit(0) }),
      },
    });

    Step("load_data", () => driver.insertSpec(accounts));

  Common builders:

    Rel.table(...)          table-level InsertSpec
    Attr.rowId()            1-based id derived from the row index
    Attr.lookup(...)        read from another generated population
    Expr.*                  arithmetic, literals, conditionals, stdlib calls
    Draw.*                  deterministic load-time distributions
    DrawRT.*                transaction-time random generators for workload code

DETERMINISM AND PARALLEL LOAD

  Each generated value is a pure function of the table seed, attribute path,
  and row index. That makes generated rows reproducible and lets drivers split
  a table into independent worker ranges.

  Workloads that support parallel load read:

    const LOAD_WORKERS = ENV("LOAD_WORKERS", 0, "Load-time worker count");

  and pass it to Rel.table({ parallelism: LOAD_WORKERS || undefined }).

  Example:

    stroppy run tpcc/tx -d pg -e LOAD_WORKERS=8 \
      --steps drop_schema,create_schema,load_data

DRIVERS

  InsertSpec is implemented by postgres, mysql, picodata, ydb, noop, and csv.
  Native mode maps to COPY for PostgreSQL, BulkUpsert for YDB, driver-native
  bulk paths where available, CSV file output for csv, and discard for noop.

  CSV is useful for reference datasets:

    stroppy run tpcb/tx -D driverType=csv \
      -D url='/tmp/tpcb-csv?merge=true&workload=tpcb' \
      --steps drop_schema,create_schema,load_data

REFERENCES

  docs/datagen-framework.md   Full workload-author guide
  docs/parallelism.md         Parallel InsertSpec contract and tuning
  workloads/simple/simple.ts  Minimal example
  workloads/tpcb/tx.ts        Small relational workload
  workloads/tpch/tx.ts        Relationship and dictionary-heavy workload
`,
	})
}
