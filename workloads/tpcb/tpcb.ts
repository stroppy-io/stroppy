import { Options } from "k6/options";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import { NotifyStep, Teardown } from "k6/x/stroppy";

import { Status, InsertMethod } from "./stroppy.pb.js";
import {
  NewDriverByConfig,
  NewGeneratorByRule as NewGenByRule,
  AB,
  G,
  InsertValues,
} from "./helpers.ts";
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
      duration: __ENV.DURATION || "1s",
    },
  },
};

// Initialize driver with GlobalConfig
const driver = NewDriverByConfig({
  driver: {
    url: __ENV.DRIVER_URL || "postgres://postgres:postgres@localhost:5432",
    driverType: 1,
    dbSpecific: {
      fields: [
        {
          type: { oneofKind: "string", string: "error" },
          key: "trace_log_level",
        },
        {
          type: { oneofKind: "string", string: "5m" },
          key: "max_conn_lifetime",
        },
        {
          type: { oneofKind: "string", string: "2m" },
          key: "max_conn_idle_time",
        },
        { type: { oneofKind: "int32", int32: 1 }, key: "max_conns" },
        { type: { oneofKind: "int32", int32: 1 }, key: "min_conns" },
        { type: { oneofKind: "int32", int32: 1 }, key: "min_idle_conns" },
      ],
    },
  },
});

const sections = parse_sql_with_groups(open(__SQL_FILE));

// Setup function: create schema and load data
export function setup() {
  NotifyStep("create_schema", Status.STATUS_RUNNING);
  sections["section cleanup"].forEach((query) =>
    driver.runQuery(query.sql, {}),
  );

  sections["section create_schema"].forEach((query) =>
    driver.runQuery(query.sql, {}),
  );
  NotifyStep("create_schema", Status.STATUS_COMPLETED);

  NotifyStep("load_data", Status.STATUS_RUNNING);
  console.log("Loading branches...");

  InsertValues(driver, {
    count: BRANCHES,
    tableName: "pgbench_branches",
    method: InsertMethod.COPY_FROM,
    params: G.params({
      bid: G.int32Seq(1, BRANCHES),
      bbalance: G.int32(0),
      filler: G.str(88, AB.en),
    }),
  });

  console.log("Loading tellers...");
  InsertValues(driver, {
    count: TELLERS,
    tableName: "pgbench_tellers",
    method: InsertMethod.COPY_FROM,
    params: G.params({
      tid: G.int32Seq(1, TELLERS),
      bid: G.int32(1, BRANCHES),
      tbalance: G.int32(0),
      filler: G.str(84, AB.en),
    }),
  });

  console.log("Loading accounts...");
  InsertValues(driver, {
    count: ACCOUNTS,
    tableName: "pgbench_accounts",
    method: InsertMethod.COPY_FROM,
    params: G.params({
      aid: G.int32Seq(1, ACCOUNTS),
      bid: G.int32(1, BRANCHES),
      abalance: G.int32(0),
      filler: G.str(84, AB.en),
    }),
    groups: [],
  });
  console.log("Data loading completed!");
  NotifyStep("load_data", Status.STATUS_COMPLETED);

  sections["section analyze"].forEach((query) =>
    driver.runQuery(query.sql, {}),
  );

  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}

// Generators for transaction parameters
const aidGen = NewGenByRule(5, G.int32(1, ACCOUNTS));
const tidGen = NewGenByRule(6, G.int32(1, TELLERS));
const bidGen = NewGenByRule(7, G.int32(1, BRANCHES));
const deltaGen = NewGenByRule(8, G.int32(-5000, 5000));

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
