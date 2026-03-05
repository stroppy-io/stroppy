import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { InsertMethod, DriverConfig_DriverType } from "./stroppy.pb.js";
import { DriverX, AB, C, R, Step, S } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

// TPC-B Configuration Constants
const SCALE_FACTOR = +(__ENV.SCALE_FACTOR || 1);
const BRANCHES = SCALE_FACTOR;
const TELLERS = 10 * SCALE_FACTOR;
const ACCOUNTS = 100000 * SCALE_FACTOR;

// K6 options
export const options: Options = {
  setupTimeout: "5h",
  scenarios: {
    tpcb_transaction: {
      executor: "constant-vus",
      exec: "tpcb_transaction",
      vus: 10,
      duration: __ENV.DURATION || "1h",
    },
  },
};

// Initialize driver with GlobalConfig
const driver = DriverX.fromConfig({
  driver: {
    url: __ENV.DRIVER_URL || "postgres://postgres:postgres@localhost:5432",
    driverType: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
    connectionType: { is: {oneofKind:"sharedPool", sharedPool: {sharedConnections: 10}}},
    dbSpecific: {
      fields: [],
    },
  },
});

const sql = parse_sql_with_sections(open(__ENV.SQL_FILE));

// Setup function: create schema and load data
export function setup() {
  Step("create_schema", () => {
    sql("cleanup").forEach((query) => driver.runQuery(query, {}));
    sql("create_schema").forEach((query) => driver.runQuery(query, {}));
  });

  Step("load_data", () => {
    driver.insert("pgbench_branches", BRANCHES, {
      method: InsertMethod.COPY_FROM,
      params: {
        bid: S.int32(1, BRANCHES),
        bbalance: C.int32(0),
        filler: R.str(88, AB.en),
      },
    });

    driver.insert("pgbench_tellers", TELLERS, {
      method: InsertMethod.COPY_FROM,
      params: {
        tid: S.int32(1, TELLERS),
        bid: R.int32(1, BRANCHES),
        tbalance: C.int32(0),
        filler: R.str(84, AB.en),
      },
    });

    driver.insert("pgbench_accounts", ACCOUNTS, {
      method: InsertMethod.COPY_FROM,
      params: {
        aid: S.int32(1, ACCOUNTS),
        bid: R.int32(1, BRANCHES),
        abalance: C.int32(0),
        filler: R.str(84, AB.en),
      },
    });

    sql("analyze").forEach((query) => driver.runQuery(query, {}));
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
export function tpcb_transaction() {
  driver.runQuery(sql("workload", "tpcb_transaction")!, {
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
