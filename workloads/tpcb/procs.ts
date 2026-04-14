import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, AB, C, R, Step, S, ENV, declareDriverSetup } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

declare const __VU: number;

// TPC-B Configuration Constants
const SCALE_FACTOR = ENV(["SCALE_FACTOR", "BRANCHES"], 1, "TPC-B scale factor");
const POOL_SIZE   = ENV("POOL_SIZE", 50, "Connection pool size");

const BRANCHES = SCALE_FACTOR;
const TELLERS  = 10 * SCALE_FACTOR;
const ACCOUNTS = 100000 * SCALE_FACTOR;

// K6 options — VUs/duration set via CLI or k6 defaults.
export const options: Options = {
  setupTimeout: String(SCALE_FACTOR) + "m",
};

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql)
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "native",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

// procs.ts targets pg + mysql only — picodata and ydb have no stored procedures.
if (driverConfig.driverType === "picodata" || driverConfig.driverType === "ydb") {
  throw new Error(
    `tpcb/procs.ts only supports postgres and mysql (got driverType=${driverConfig.driverType}). ` +
    `Use tpcb/tx.ts for picodata/ydb.`,
  );
}

const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql:    "./mysql.sql",
};
const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file path (defaults per driverType)")
  ?? _sqlByDriver[driverConfig.driverType!]
  ?? "./pg.sql";

const driver = DriverX.create().setup(driverConfig);

const sql = parse_sql_with_sections(open(SQL_FILE));

// Setup function: drop, create schema + procs, load data
export function setup() {
  Step("drop_schema", () => {
    sql("drop_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_schema", () => {
    sql("create_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_procedures", () => {
    sql("create_procedures").forEach((query) => driver.exec(query, {}));
  });

  Step("load_data", () => {
    driver.insert("pgbench_branches", BRANCHES, {
      params: {
        bid: S.int32(1, BRANCHES),
        bbalance: C.int32(0),
        filler: R.str(88, AB.en),
      },
    });

    driver.insert("pgbench_tellers", TELLERS, {
      params: {
        tid: S.int32(1, TELLERS),
        bid: R.int32(1, BRANCHES),
        tbalance: C.int32(0),
        filler: R.str(84, AB.en),
      },
    });

    driver.insert("pgbench_accounts", ACCOUNTS, {
      params: {
        aid: S.int32(1, ACCOUNTS),
        bid: R.int32(1, BRANCHES),
        abalance: C.int32(0),
        filler: R.str(84, AB.en),
      },
    });
  });

  Step.begin("workload");
  return;
}

// Generators for transaction parameters
const aidGen   = R.int32(1, ACCOUNTS).gen();
const tidGen   = R.int32(1, TELLERS).gen();
const bidGen   = R.int32(1, BRANCHES).gen();
const deltaGen = R.int32(-5000, 5000).gen();

// Per-VU monotonic counter for history PK (uniform across all dialects).
let hcounter = (typeof __VU === "number" ? __VU : 1) * 1_000_000_000;
const nextHid = () => ++hcounter;

// TPC-B transaction workload — single stored proc call per iteration.
export default function (): void {
  driver.exec(sql("workload_procs", "tpcb_transaction")!, {
    p_aid: aidGen.next(),
    p_tid: tidGen.next(),
    p_bid: bidGen.next(),
    p_delta: deltaGen.next(),
    p_hid: nextHid(),
  });
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
