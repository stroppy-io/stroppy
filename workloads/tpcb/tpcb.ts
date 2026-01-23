import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;
import {
  NewDriverByConfig,
  NotifyStep,
  Teardown,
  NewGeneratorByRuleBin,
} from "k6/x/stroppy";

import { Options } from "k6/options";
import {
  GlobalConfig,
  Status,
  Generation_Rule,
  InsertDescriptor,
} from "./stroppy.pb.js";
import { parse_sql_with_groups } from "./parse_sql_2.js";

// declare const __ENV: Record<string, string | undefined>;
// declare const __SQL_FILE: string;

// TPC-B Configuration Constants
const SCALE_FACTOR = +(__ENV.SCALE_FACTOR || 1);
const BRANCHES = SCALE_FACTOR;
const TELLERS = 10 * SCALE_FACTOR;
const ACCOUNTS = 100000 * SCALE_FACTOR;

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
const driver = NewDriverByConfig(
  GlobalConfig.toBinary(
    GlobalConfig.create({
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
    }),
  ),
);

const sections = parse_sql_with_groups(open(__SQL_FILE));

// Create generators for data loading
const branchIdGen = NewGeneratorByRule(
  0,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: 1, max: BRANCHES } },
    unique: true,
  }),
);

const tellerIdGen = NewGeneratorByRule(
  1,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: 1, max: TELLERS } },
    unique: true,
  }),
);

const accountIdGen = NewGeneratorByRule(
  2,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: 1, max: ACCOUNTS } },
    unique: true,
  }),
);

const branchForTellerGen = NewGeneratorByRule(
  3,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: 1, max: BRANCHES } },
  }),
);

const branchForAccountGen = NewGeneratorByRule(
  4,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: 1, max: BRANCHES } },
  }),
);

// Generators for filler strings
const branchFillerGen = NewGeneratorByRule(
  9,
  Generation_Rule.create({
    kind: {
      oneofKind: "stringRange",
      stringRange: { minLen: "88", maxLen: "88" },
    },
  }),
);

const tellerFillerGen = NewGeneratorByRule(
  10,
  Generation_Rule.create({
    kind: {
      oneofKind: "stringRange",
      stringRange: { minLen: "84", maxLen: "84" },
    },
  }),
);

const accountFillerGen = NewGeneratorByRule(
  11,
  Generation_Rule.create({
    kind: {
      oneofKind: "stringRange",
      stringRange: { minLen: "84", maxLen: "84" },
    },
  }),
);

// Setup function: create schema and load data
export function setup() {
  NotifyStep("create_schema", Status.STATUS_RUNNING);

  // Drop tables if they exist
  sections["section cleanup"].forEach((query) =>
    driver.runQuery(query.sql, {}),
  );

  // Create pgbench_branches table
  sections["section create_schema"].forEach((query) =>
    driver.runQuery(query.sql, {}),
  );

  NotifyStep("create_schema", Status.STATUS_COMPLETED);

  NotifyStep("load_data", Status.STATUS_RUNNING);

  if (true) {
    // TODO: port inserts to query like api, or just drop proto
    console.log("Loading branches...");
    const branchInsert = InsertDescriptor.create({
      name: "insert_branches",
      tableName: "pgbench_branches",
      method: 1,
      params: [
        {
          name: "bid",
          generationRule: {
            kind: {
              oneofKind: "int32Range",
              int32Range: { max: BRANCHES, min: 1 },
            },
            unique: true,
          },
        },
        {
          name: "bbalance",
          generationRule: {
            kind: { oneofKind: "int32Const", int32Const: 0 },
          },
        },
        {
          name: "filler",
          generationRule: {
            kind: {
              oneofKind: "stringRange",
              stringRange: {
                maxLen: "88",
                minLen: "88",
              },
            },
          },
        },
      ],
      groups: [],
    });
    driver.insertValues(InsertDescriptor.toBinary(branchInsert), BRANCHES);

    console.log("Loading tellers...");
    // Insert tellers
    const tellerInsert = InsertDescriptor.create({
      name: "insert_tellers",
      tableName: "pgbench_tellers",
      method: 1,
      params: [
        {
          name: "tid",
          generationRule: {
            kind: {
              oneofKind: "int32Range",
              int32Range: { max: TELLERS, min: 1 },
            },
            unique: true,
          },
        },
        {
          name: "bid",
          generationRule: {
            kind: {
              oneofKind: "int32Range",
              int32Range: { max: BRANCHES, min: 1 },
            },
          },
        },
        {
          name: "tbalance",
          generationRule: {
            kind: { oneofKind: "int32Const", int32Const: 0 },
          },
        },
        {
          name: "filler",
          generationRule: {
            kind: {
              oneofKind: "stringRange",
              stringRange: {
                maxLen: "84",
                minLen: "84",
              },
            },
          },
        },
      ],
      groups: [],
    });
    driver.insertValues(InsertDescriptor.toBinary(tellerInsert), TELLERS);

    console.log("Loading accounts...");
    // Insert accounts
    const accountInsert = InsertDescriptor.create({
      name: "insert_accounts",
      tableName: "pgbench_accounts",
      method: 1,
      params: [
        {
          name: "aid",
          generationRule: {
            kind: {
              oneofKind: "int32Range",
              int32Range: { max: ACCOUNTS, min: 1 },
            },
            unique: true,
          },
        },
        {
          name: "bid",
          generationRule: {
            kind: {
              oneofKind: "int32Range",
              int32Range: { max: BRANCHES, min: 1 },
            },
          },
        },
        {
          name: "abalance",
          generationRule: {
            kind: { oneofKind: "int32Const", int32Const: 0 },
          },
        },
        {
          name: "filler",
          generationRule: {
            kind: {
              oneofKind: "stringRange",
              stringRange: {
                maxLen: "84",
                minLen: "84",
              },
            },
          },
        },
      ],
      groups: [],
    });
    driver.insertValues(InsertDescriptor.toBinary(accountInsert), ACCOUNTS);
  } else {
    // so slow...
    // Insert branches
    console.log("Loading branches...");
    for (let i = 0; i < BRANCHES; i++) {
      driver.runQuery(
        "INSERT INTO pgbench_branches (bid, bbalance, filler) VALUES (:branch, 0, :filler)",
        { branch: branchIdGen.next(), filler: branchFillerGen.next() },
      );
    }

    console.log("Loading tellers...");
    // Insert tellers
    for (let i = 0; i < TELLERS; i++) {
      driver.runQuery(
        "INSERT INTO pgbench_tellers (tid, bid, tbalance, filler) VALUES (:tid, :bid, 0, :filler)",
        {
          tid: tellerIdGen.next(),
          bid: branchForTellerGen.next(),
          filler: tellerFillerGen.next(),
        },
      );
    }

    console.log("Loading accounts...");
    // Insert accounts
    for (let i = 0; i < ACCOUNTS; i++) {
      driver.runQuery(
        "INSERT INTO pgbench_accounts (aid, bid, abalance, filler) VALUES (:aid, :bid, 0, :filler)",
        {
          aid: accountIdGen.next(),
          bid: branchForAccountGen.next(),
          filler: accountFillerGen.next(),
        },
      );
    }
  }

  console.log("Data loading completed!");
  NotifyStep("load_data", Status.STATUS_COMPLETED);

  // Analyze tables
  sections["section analyze"].forEach((query) =>
    driver.runQuery(query.sql, {}),
  );

  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}

// Generators for transaction parameters
const aidGen = NewGeneratorByRule(
  5,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: 1, max: ACCOUNTS } },
  }),
);

const tidGen = NewGeneratorByRule(
  6,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: 1, max: TELLERS } },
  }),
);

const bidGen = NewGeneratorByRule(
  7,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: 1, max: BRANCHES } },
  }),
);

const deltaGen = NewGeneratorByRule(
  8,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: -5000, max: 5000 } },
  }),
);

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
