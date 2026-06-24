import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, Step, ENV, GlobalOnce, declareDriverSetup } from "./helpers.ts";
import { parse_sql, parse_sql_with_sections } from "./parse_sql.js";

// Backend query set (the 99 TPC-DS queries), unchanged. Set automatically when a
// .sql file is passed as the run argument.
const SQL_FILE = ENV("SQL_FILE", "", "Path to TPC-DS query SQL file");
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

export const options: Options = {
  vus: 1,
  iterations: 1,
};

const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
});

const driver = DriverX.create().setup(driverConfig);

// Schema DDL (one "create_schema" section) and the read-only query set, read at
// module init like the TPC-H workload.
const schema = parse_sql_with_sections(open("./schema.pg.sql"));
const queries = parse_sql(open(SQL_FILE));

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
