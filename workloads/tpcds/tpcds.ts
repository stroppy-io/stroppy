import { Options } from "k6/options";
import { Teardown, GenerateTpcdsQueries } from "k6/x/stroppy";
import { DriverX, Step, ENV, GlobalOnce, declareDriverSetup } from "./helpers.ts";
import { parse_sql, parse_sql_with_sections } from "./parse_sql.js";

// Data generation: the ported dsdgen generator owns it; we pass table + scale.
const SCALE_FACTOR = Number(
  ENV("SCALE_FACTOR", "1", "TPC-DS scale factor; fractional allowed for smoke tests"),
);
const LOAD_WORKERS = ENV(
  "LOAD_WORKERS",
  0,
  "Load-time worker count per table (0 = framework default)",
) as number;

if (!Number.isFinite(SCALE_FACTOR) || SCALE_FACTOR <= 0) {
  throw new Error(`SCALE_FACTOR must be a positive number, got ${SCALE_FACTOR}`);
}

// A full load + single query pass at large scale far exceeds k6's default 10m
// cap, so the workload sets its own. Override with MAX_DURATION if needed.
const MAX_DURATION = ENV("MAX_DURATION", "24h", "Max wall-clock for the run (k6 duration)");

// Table load order: dimensions and static tables first, fan-out fact tables
// last (each returns table after its parent sales table).
const TPCDS_TABLES = [
  "income_band", "ship_mode", "reason", "household_demographics",
  "customer_demographics", "date_dim", "time_dim", "warehouse",
  "web_page", "web_site", "catalog_page", "customer_address",
  "customer", "call_center", "store", "promotion", "item", "inventory",
  "store_sales", "store_returns", "catalog_sales", "catalog_returns",
  "web_sales", "web_returns",
];

// One shared iteration. Declared as a scenario (not the vus/iterations
// shorthand) so maxDuration can lift k6's 10m default for large-scale loads.
export const options: Options = {
  scenarios: {
    tpcds: {
      executor: "shared-iterations",
      vus: 1,
      iterations: 1,
      maxDuration: MAX_DURATION,
    },
  },
};

const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
});

const driver = DriverX.create().setup(driverConfig);

// The 99 TPC-DS queries, generated per dialect from the official query templates
// at the canonical qualification parameters (see workloads/tpcds/README or the
// dsqgen port). Picked by driverType; SQL_FILE overrides. Dialects without their
// own file fall back to pg.sql.
const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql: "./mysql.sql",
};
const _schemaByDriver: Record<string, string> = {
  postgres: "./schema.pg.sql",
  mysql: "./schema.mysql.sql",
};
const SQL_FILE =
  ENV("SQL_FILE", "", "Path to TPC-DS query SQL file (defaults per driverType)") ||
  _sqlByDriver[driverConfig.driverType!] ||
  "./pg.sql";
const SCHEMA_FILE =
  _schemaByDriver[driverConfig.driverType!] || "./schema.pg.sql";

// Query source. Default: the baked canonical (qualification) query set for the
// driver. If QUERY_STREAM is set, generate that stream's parameters in-process
// (no offline step) — valid, scale-correct, varied per seed.
const QUERY_STREAM = ENV(
  "QUERY_STREAM",
  "",
  "Generate query stream N in-process (empty = baked canonical set)",
);
const QUERY_SEED = Number(
  ENV("QUERY_SEED", "19620718", "RNG seed for generated query streams"),
);

// Schema DDL (one "create_schema" section) and the read-only query set, read at
// module init like the TPC-H workload.
const schema = parse_sql_with_sections(open(SCHEMA_FILE));
const queries: () => Array<{ name: string }> =
  QUERY_STREAM !== ""
    ? (() => {
        const gen = GenerateTpcdsQueries(
          driverConfig.driverType ?? "postgres",
          SCALE_FACTOR,
          QUERY_SEED,
          Number(QUERY_STREAM),
        );
        return () => gen;
      })()
    : parse_sql(open(SQL_FILE));

// prepareDatabase creates the schema, then generates and bulk-loads every table
// with the ported dsdgen generator. Runs once per process via GlobalOnce.
function prepareDatabase(): void {
  Step("create_schema", () => {
    const stmts = schema("create_schema");
    if (stmts) {
      stmts.forEach((q) => driver.exec(q, {}));
    }
  });

  Step("load_data", () => {
    // Ported dsdgen: the Go side owns generation; pass table + scale factor.
    for (const table of TPCDS_TABLES) {
      driver.insertTpcds(table, SCALE_FACTOR, LOAD_WORKERS);
    }
  });

  Step.begin("workload");
}

export function setup(): void {
  return;
}

export default function (): void {
  GlobalOnce("tpcds.prepare", prepareDatabase);

  Step("queries", () => {
    queries().forEach((query) => {
      console.log(`tpc-ds-like: ${query.name}`);
      driver.exec(query, {});
    });
  });
}

export function teardown(): void {
  Step.end("workload");
  Teardown();
}
