import { Options } from "k6/options";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import { NotifyStep, Teardown } from "k6/x/stroppy";

import { Status, InsertMethod, DriverConfig_DriverType } from "./stroppy.pb.js";
import { DriverX, NewGen, AB, R, Step, S } from "./helpers.ts";
import { parse_sql_with_groups } from "./parse_sql.js";

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
    dbSpecific: {
      fields: [],
    },
  },
});

const sections = parse_sql_with_groups(open(__SQL_FILE));

// Setup function: create schema and load data
export function setup() {
  Step("create_schema", () => {
    sections["section cleanup"].forEach((query) => driver.runQuery(query, {}));

    sections["section create_schema"].forEach((query) =>
      driver.runQuery(query, {}),
    );
  });

  Step("load_data", () => {
    driver.insert("pgbench_branches", BRANCHES, {
      method: InsertMethod.COPY_FROM,
      params: {
        bid: S.int32(1, BRANCHES),
        bbalance: R.int32(0),
        filler: R.str(88, AB.en),
      },
    });

    driver.insert("pgbench_tellers", TELLERS, {
      method: InsertMethod.COPY_FROM,
      params: {
        tid: S.int32(1, TELLERS),
        bid: R.int32(1, BRANCHES),
        tbalance: R.int32(0),
        filler: R.str(84, AB.en),
      },
    });

    driver.insert("pgbench_accounts", ACCOUNTS, {
      method: InsertMethod.COPY_FROM,
      params: {
        aid: S.int32(1, ACCOUNTS),
        bid: R.int32(1, BRANCHES),
        abalance: R.int32(0),
        filler: R.str(84, AB.en),
      },
    });

    sections["section analyze"].forEach((query) => driver.runQuery(query, {}));
  });

  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}

// Generators for transaction parameters
const aidGen = NewGen(5, R.int32(1, ACCOUNTS));
const tidGen = NewGen(6, R.int32(1, TELLERS));
const bidGen = NewGen(7, R.int32(1, BRANCHES));
const deltaGen = NewGen(8, R.int32(-5000, 5000));

// TPC-B transaction workload
export function tpcb_transaction() {
  driver.runQuery("SELECT tpcb_transaction(:p_aid, :p_tid, :p_bid, :p_delta)", {
    p_aid: aidGen.next(),
    p_tid: tidGen.next(),
    p_bid: bidGen.next(),
    p_delta: deltaGen.next(),
  });
}

export function teardown() {
  NotifyStep("workload", Status.STATUS_COMPLETED);
  Teardown();
}
