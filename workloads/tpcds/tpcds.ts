import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;
import stroppy from "k6/x/stroppy";

import { Options } from "k6/options";
import { GlobalConfig, Status } from "./stroppy.pb.js";
import { parse_sql } from "./parse_sql_2.js";

export const options: Options = {
  setupTimeout: "5m",
  scenarios: {
    workload: {
      executor: "shared-iterations",
      exec: "workload",
      vus: 1,
      iterations: 1,
    },
  },
};

// Sql Driver interface
// is an interface of stroppy go module
interface Driver {
  runQuery(sql: string, args: Record<string, any>): void; // TODO: return value, is it posible to make it generic?
  teardown(): any; // error // TODO: proper error type
  notifyStep(name: String, status: Status): void;
  // TODO: make a global function
  defineConfig(config: GlobalConfig): void;
  // TODO: delete
  // deprecated
  defineConfigBin(config: GlobalConfig): void;
}

const driver: Driver = stroppy;

if (!globalThis.defineConfig) {
  globalThis.defineConfig = function (config: GlobalConfig): void {
    driver.defineConfig(config);
  };
}

declare const __ENV: Record<string, string | undefined>;

// Initialize driver with GlobalConfig
defineConfig({
  driver: {
    url:
      __ENV.DRIVER_URL ||
      "postgres://arenadev:arenadev@localhost:5432/postgres?search_path=tpcds,public",
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
  version: "",
  runId: "",
  seed: "",
  metadata: {},
});

declare function open(path: string): string; // k6 function to get files content

const content: string = open("query_0.sql"); // TODO: push file name trought go cli argument
const parsedQueries = parse_sql(content);

export function setup() {
  driver.notifyStep("workload", Status.STATUS_RUNNING);
  return;
}

export function workload() {
  for (const query of parsedQueries) {
    // TODO: add statistics and etc
    console.log(`tpc-ds-like: ${query.name}`);
    driver.runQuery(query.sql, {});
  }
}

export function teardown() {
  driver.notifyStep("workload", Status.STATUS_COMPLETED);
  driver.teardown();
}
