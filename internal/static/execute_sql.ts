import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;
import stroppy from "k6/x/stroppy";

import { Options } from "k6/options";
import {
  UnitDescriptor,
  DriverTransactionStat,
  DriverConfig,
  WorkloadDescriptor,
  Status,
  TxIsolationLevel,
} from "./stroppy.pb.js";

import { parse_sql, update_with_sql } from "./parse_sql.ts";

export const options: Options = {
  setupTimeout: "5m",
  scenarios: {
    workload: {
      executor: "shared-iterations",
      exec: "workload",
      vus: 1,
      iterations: 1,
    },
    // workload: {
    //   executor: "constant-vus",
    //   exec: "workload",
    //   vus: 10,
    //   duration: "5m",
    // },
  },
};

// protobuf serialized messages
type BinMsg<_T extends any> = Uint8Array;

// Sql Driver interface
interface Driver {
  runUnit(unit: BinMsg<UnitDescriptor>): BinMsg<DriverTransactionStat>;
  teardown(): any; // error
  notifyStep(name: String, status: Status): void;
}
const driver: Driver = stroppy;

function RunUnit(unit: UnitDescriptor): void {
  driver.runUnit(UnitDescriptor.toBinary(unit));
}

function RunWorkload(wl: WorkloadDescriptor) {
  wl.units
    .map((wu) => wu.descriptor)
    .filter((d) => d !== undefined)
    .forEach((d) => RunUnit(d));
}

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
              params: [
                {
                  name: "accounts.id",
                  generationRule: {
                    unique: true,
                    kind: {
                      oneofKind: "int32Range",
                      int32Range: { min: 1, max: MAX_ACCOUNTS },
                    },
                  },
                },
                {
                  name: "accounts.balance",
                  generationRule: {
                    unique: false,
                    kind: {
                      oneofKind: "int32Range",
                      int32Range: { min: 1000, max: 10000 },
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
                  name: "accounts.id",
                  generationRule: {
                    unique: false,
                    kind: {
                      oneofKind: "int32Range",
                      int32Range: { min: 1, max: MAX_ACCOUNTS },
                    },
                  },
                },
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

// Load and parse SQL file, then update workloads with SQL
const sqlContent = open("tpcb_mini.sql");
const parsedWorkloads = parse_sql(sqlContent);
update_with_sql(workloads, parsedWorkloads);

// Init context: each VU gets its own driver with single connection
stroppy.parseConfig(
  DriverConfig.toBinary(
    DriverConfig.create({
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
    }),
  ),
);

// Helper to find workload by name
function getWorkload(name: string): WorkloadDescriptor | undefined {
  return workloads.find((w) => w.name === name);
}

// Setup function: create schema and load data
export function setup() {
  const createSchema = getWorkload("create_schema");
  if (createSchema) {
    driver.notifyStep("create_schema", Status.STATUS_RUNNING);
    RunWorkload(createSchema);
    driver.notifyStep("create_schema", Status.STATUS_COMPLETED);
  }

  const insert = getWorkload("insert");
  if (insert) {
    driver.notifyStep("insert", Status.STATUS_RUNNING);
    RunWorkload(insert);
    driver.notifyStep("insert", Status.STATUS_COMPLETED);
  }

  driver.notifyStep("workload", Status.STATUS_RUNNING);
  return;
}

// Main workload function
export function workload() {
  const wl = getWorkload("workload");
  if (wl) {
    RunWorkload(wl);
  }
}

export function teardown() {
  driver.notifyStep("workload", Status.STATUS_COMPLETED);

  const cleanup = getWorkload("cleanup");
  if (cleanup) {
    driver.notifyStep("cleanup", Status.STATUS_RUNNING);
    RunWorkload(cleanup);
    driver.notifyStep("cleanup", Status.STATUS_COMPLETED);
  }

  driver.teardown();
}
