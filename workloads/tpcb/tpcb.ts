import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverConfig_DriverType } from "./stroppy.pb.js";
import { DriverX, AB, C, R, Step, S, ENV } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

const SQL_FILE = ENV("SQL_FILE", "./tpcb.sql", "Path to SQL file (automatically set if .sql file provided as argument)");

// TPC-B Configuration Constants
const SCALE_FACTOR = ENV(["SCALE_FACTOR", "BRANCHES"], 1, "TPC-B scale factor");
const BRANCHES = SCALE_FACTOR;
const TELLERS = 10 * SCALE_FACTOR;
const ACCOUNTS = 100000 * SCALE_FACTOR;

// K6 options
export const options: Options = {
  setupTimeout: String(SCALE_FACTOR) + "m" ,
};

// Initialize driver with GlobalConfig
const driver = DriverX.fromConfig({
  driver: {
    url: ENV("DRIVER_URL", "postgres://postgres:postgres@localhost:5432", "Database connection URL"),
    driverType: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
    connectionType: { is: {oneofKind:"sharedPool", sharedPool: {sharedConnections: 10}}},
    dbSpecific: {
      fields: [],
    },
  },
});

const sql = parse_sql_with_sections(open(SQL_FILE));

// Setup function: create schema and load data
export function setup() {
  Step("cleanup", () => {
    sql("cleanup").forEach((query) => driver.exec(query, {}));
  })

  Step("create_schema", () => {
    sql("create_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("load_data", () => {
    driver.insert("pgbench_branches", BRANCHES, {
      method: "copy_from",
      params: {
        bid: S.int32(1, BRANCHES),
        bbalance: C.int32(0),
        filler: R.str(88, AB.en),
      },
    });

    driver.insert("pgbench_tellers", TELLERS, {
      method: "copy_from",
      params: {
        tid: S.int32(1, TELLERS),
        bid: R.int32(1, BRANCHES),
        tbalance: C.int32(0),
        filler: R.str(84, AB.en),
      },
    });

    driver.insert("pgbench_accounts", ACCOUNTS, {
      method: "copy_from",
      params: {
        aid: S.int32(1, ACCOUNTS),
        bid: R.int32(1, BRANCHES),
        abalance: C.int32(0),
        filler: R.str(84, AB.en),
      },
    });

    sql("analyze").forEach((query) => driver.exec(query, {}));
  });

  Step.begin("workload");
  return;
}

// Generators for transaction parameters
const aidGen = R.int32(1, ACCOUNTS).gen();
const tidGen = R.int32(1, TELLERS).gen();
const bidGen = R.int32(1, BRANCHES).gen();
const deltaGen = R.int32(-5000, 5000).gen();

// TPC-B transaction workload
export default function (): void {
  driver.exec(sql("workload", "tpcb_transaction")!, {
    p_aid: aidGen.next(),
    p_tid: tidGen.next(),
    p_bid: bidGen.next(),
    p_delta: deltaGen.next(),
  });
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
