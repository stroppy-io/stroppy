import { Options } from "k6/options";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import { NotifyStep, Teardown } from "k6/x/stroppy";

import { Status } from "./stroppy.pb.js";
import { NewDriverByConfig } from "./helpers.ts";
import { parse_sql } from "./parse_sql.js";

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
  version: "",
  runId: "",
  seed: "0",
  metadata: {},
});

declare function open(path: string): string; // k6 function to get files content

const parsedQueries = parse_sql(open(__SQL_FILE));

export function setup() {
  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}

export function workload() {
  parsedQueries.forEach((query) => {
    // TODO: add statistics and etc
    console.log(`tpc-ds-like: ${query.name}`);
    driver.runQuery(query.sql, {});
  });
}

export function teardown() {
  NotifyStep("workload", Status.STATUS_COMPLETED);
  Teardown();
}
