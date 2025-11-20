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
  InsertDescriptor,
  Status,
} from "./stroppy.pb.js";

export const options: Options = {
  setupTimeout: "5h",
  scenarios: {
    tpcb_transaction: {
      executor: "constant-vus",
      exec: "tpcb_transaction",
      vus: 10,
      duration: "1h",
    },
  },
};

// TPC-B Configuration Constants
const SCALE_FACTOR = 10000;
const BRANCHES = SCALE_FACTOR;
const TELLERS = 10 * SCALE_FACTOR;
const ACCOUNTS = 100000 * SCALE_FACTOR;

// protobuf serialized messages
type BinMsg<_T extends any> = Uint8Array;

// Sql Driver interface
interface Driver {
  runUnit(unit: BinMsg<UnitDescriptor>): BinMsg<DriverTransactionStat>;
  insertValues(
    insert: BinMsg<InsertDescriptor>,
    count: number,
  ): BinMsg<DriverTransactionStat>;
  teardown(): any; // error
  notifyStep(name: String, status: Status): void;
}
const driver: Driver = stroppy;

function RunUnit(unit: UnitDescriptor): void {
  driver.runUnit(UnitDescriptor.toBinary(unit));
}

function RunUnitBin(unit: BinMsg<UnitDescriptor>): void {
  driver.runUnit(unit);
}

function RunWorkload(wl: WorkloadDescriptor) {
  wl.units
    .map((wu) => wu.descriptor)
    .filter((d) => d !== undefined)
    .forEach((d) => RunUnit(d));
}

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

// Setup function: create schema and load data
export function setup() {
  driver.notifyStep("create_schema", Status.STATUS_RUNNING);

  const workload = WorkloadDescriptor.create({
    name: "tpcb_setup",
    units: [
      // Drop tables if they exist
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "drop_transaction",
              sql: "DROP FUNCTION IF EXISTS tpcb_transaction",
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
              name: "drop_history",
              sql: "DROP TABLE IF EXISTS pgbench_history CASCADE",
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
              name: "drop_accounts",
              sql: "DROP TABLE IF EXISTS pgbench_accounts CASCADE",
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
              name: "drop_tellers",
              sql: "DROP TABLE IF EXISTS pgbench_tellers CASCADE",
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
              name: "drop_branches",
              sql: "DROP TABLE IF EXISTS pgbench_branches CASCADE",
              params: [],
              groups: [],
            },
          },
        },
      },
      // Create pgbench_branches table
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "pgbench_branches",
              tableIndexes: [],
              columns: [
                { name: "bid", sqlType: "INTEGER", primaryKey: true },
                { name: "bbalance", sqlType: "INTEGER" },
                { name: "filler", sqlType: "CHAR(88)" },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      // Create pgbench_tellers table
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "pgbench_tellers",
              tableIndexes: [],
              columns: [
                { name: "tid", sqlType: "INTEGER", primaryKey: true },
                { name: "bid", sqlType: "INTEGER" },
                { name: "tbalance", sqlType: "INTEGER" },
                { name: "filler", sqlType: "CHAR(84)" },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      // Create pgbench_accounts table
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "pgbench_accounts",
              tableIndexes: [],
              columns: [
                { name: "aid", sqlType: "INTEGER", primaryKey: true },
                { name: "bid", sqlType: "INTEGER" },
                { name: "abalance", sqlType: "INTEGER" },
                { name: "filler", sqlType: "CHAR(84)" },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      // Create pgbench_history table
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "pgbench_history",
              tableIndexes: [],
              columns: [
                { name: "tid", sqlType: "INTEGER" },
                { name: "bid", sqlType: "INTEGER" },
                { name: "aid", sqlType: "INTEGER" },
                { name: "delta", sqlType: "INTEGER" },
                { name: "mtime", sqlType: "TIMESTAMP" },
                { name: "filler", sqlType: "CHAR(22)" },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      // Create indexes
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "create_accounts_idx",
              sql: "CREATE INDEX pgbench_accounts_bid_idx ON pgbench_accounts (bid)",
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
              name: "create_tellers_idx",
              sql: "CREATE INDEX pgbench_tellers_bid_idx ON pgbench_tellers (bid)",
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
              name: "tpcb_transaction",
              sql: `CREATE OR REPLACE FUNCTION tpcb_transaction(
    p_aid INTEGER,
    p_tid INTEGER,
    p_bid INTEGER,
    p_delta INTEGER
)
RETURNS INTEGER
LANGUAGE plpgsql
AS $$
DECLARE
    v_balance INTEGER;
BEGIN
    -- Update account balance
    UPDATE pgbench_accounts
    SET abalance = abalance + p_delta
    WHERE pgbench_accounts.aid = p_aid;

    -- Get the updated account balance
    SELECT pgbench_accounts.abalance INTO v_balance
    FROM pgbench_accounts
    WHERE pgbench_accounts.aid = p_aid;

    -- Update teller balance
    UPDATE pgbench_tellers
    SET tbalance = tbalance + p_delta
    WHERE pgbench_tellers.tid = p_tid;

    -- Update branch balance
    UPDATE pgbench_branches
    SET bbalance = bbalance + p_delta
    WHERE pgbench_branches.bid = p_bid;

    -- Insert history record
    INSERT INTO pgbench_history (tid, bid, aid, delta, mtime, filler)
    VALUES (p_tid, p_bid, p_aid, p_delta, CURRENT_TIMESTAMP, 'tpcb_tx');

    RETURN v_balance;
END;
$$;
`,
              params: [],
              groups: [],
            },
          },
        },
      },
    ],
  });

  // Run schema creation
  RunWorkload(workload);
  driver.notifyStep("create_schema", Status.STATUS_COMPLETED);

  driver.notifyStep("load_data", Status.STATUS_RUNNING);
  // Insert branches
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

  console.log("Data loading completed!");
  driver.notifyStep("load_data", Status.STATUS_COMPLETED);

  // Analyze tables
  const analyzeWorkload = WorkloadDescriptor.create({
    name: "analyze_tables",
    units: [
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "vacuum_analyze_branches",
              sql: "VACUUM ANALYZE pgbench_branches",
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
              name: "vacuum_analyze_tellers",
              sql: "VACUUM ANALYZE pgbench_tellers",
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
              name: "vacuum_analyze_accounts",
              sql: "VACUUM ANALYZE pgbench_accounts",
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
              name: "vacuum_analyze_history",
              sql: "VACUUM ANALYZE pgbench_history",
              params: [],
              groups: [],
            },
          },
        },
      },
    ],
  });
  RunWorkload(analyzeWorkload);

  driver.notifyStep("workload", Status.STATUS_RUNNING);
  return;
}

// TPC-B transaction workload
const tpcbTransactionDescriptorBin: BinMsg<UnitDescriptor> =
  UnitDescriptor.toBinary({
    type: {
      oneofKind: "query",
      query: {
        name: "tpcb_transaction",
        sql: "select tpcb_transaction(${aid[1:ACCOUNTS]}, ${tid}, ${bid}, ${delta});",
        params: [
          {
            name: "aid",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: { max: ACCOUNTS, min: 1 },
              },
            },
          },
          {
            name: "tid",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: { max: TELLERS, min: 1 },
              },
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
            name: "delta",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: { max: 5000, min: -5000 },
              },
            },
          },
        ],
        groups: [],
      },
    },
  });

export function tpcb_transaction() {
  RunUnitBin(tpcbTransactionDescriptorBin);
}

export function teardown() {
  driver.notifyStep("workload", Status.STATUS_COMPLETED);
  driver.teardown();
}
