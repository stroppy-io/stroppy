import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;
import stroppy from "k6/x/stroppy";

import { Options } from "k6/options";
import {
  DriverConfig,
  GlobalConfig,
  WorkloadDescriptor,
  Status,
  TxIsolationLevel,
} from "./stroppy.pb.js";

import { parse_sql, update_with_sql } from "./parse_sql.ts";
import {
  Driver,
  RunWorkload,
  getWorkload,
  runWorkloadStep,
  lookup,
} from "./helpers.ts";
import { params_by_ddl } from "./analyze_ddl.js";
import { apply_generators_ranges } from "./apply_generators.ts";

export const options: Options = {
  setupTimeout: "5m",
  scenarios: {
    workload: {
      executor: "constant-vus",
      exec: "workload",
      vus: 10,
      duration: "1s",
    },
  },
};

const driver: Driver = stroppy;

// TODO: inject such kind of constants to generators system
const MAX_ACCOUNTS = 100000;

// Define workload descriptors
const workloads: WorkloadDescriptor[] = [
  WorkloadDescriptor.create({
    name: "create_schema",
    units: [
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "create_accounts_table",
              sql: "",
              params: [],
              groups: [],
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "create_history_table",
              sql: "",
              params: [],
              groups: [],
            },
          },
        },
      },
    ],
  }),
  WorkloadDescriptor.create({
    name: "insert",
    units: [
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "insert_accounts",
              sql: "",
              groups: [],
              params: [],
            },
          },
        },
      },
    ],
  }),
  WorkloadDescriptor.create({
    name: "workload",
    units: [
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "transaction",
            transaction: {
              name: "update_and_log",
              isolationLevel: TxIsolationLevel.UNSPECIFIED,
              queries: [
                { name: "update_balance", sql: "", params: [], groups: [] },
                { name: "insert_history", sql: "", params: [], groups: [] },
              ],
              groups: [],
              params: [
                {
                  name: "amount",
                  generationRule: {
                    unique: false,
                    kind: {
                      oneofKind: "int32Range",
                      int32Range: { min: -1000, max: 1000 },
                    },
                  },
                },
              ],
            },
          },
        },
      },
    ],
  }),
  WorkloadDescriptor.create({
    name: "cleanup",
    units: [
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: { name: "drop_tables", sql: "", params: [], groups: [] },
          },
        },
      },
    ],
  }),
];

declare const __SQL_FILE: string;

// Load and parse SQL file, then update workloads with SQL
const sqlContent = open(__SQL_FILE);
const parsedWorkloads = parse_sql(sqlContent);
update_with_sql(workloads, parsedWorkloads);

// Apply default generators
lookup(workloads, "insert", "query", "insert_accounts").params.push(
  ...params_by_ddl(workloads, "create_schema", "accounts"),
);

lookup(workloads, "workload", "transaction", "update_and_log").params.push(
  ...params_by_ddl(workloads, "create_schema", "accounts"),
);

// Apply generator ranges from SQL syntax
apply_generators_ranges(workloads);

declare const __ENV: Record<string, string | undefined>;

// Initialize driver with GlobalConfig
// This is called at the top level to configure the driver
stroppy.defineConfig(
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
);

// Setup function: create schema and load data
export function setup() {
  try {
    runWorkloadStep(driver, workloads, "cleanup");
  } catch (e) {
    // Ignore cleanup error if tables don't exist
  }
  runWorkloadStep(driver, workloads, "create_schema");
  runWorkloadStep(driver, workloads, "insert");
  driver.notifyStep("workload", Status.STATUS_RUNNING);
  return;
}

// Main workload function
export function workload() {
  const wl = getWorkload(workloads, "workload");
  if (wl) {
    RunWorkload(driver, wl);
  }
}

export function teardown() {
  driver.notifyStep("workload", Status.STATUS_COMPLETED);
  runWorkloadStep(driver, workloads, "cleanup");
  driver.teardown();
}
