import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;
import stroppy from "k6/x/stroppy";

import { Options } from "k6/options";
import { GlobalConfig, Status } from "./stroppy.pb.js";
import { parse_sql } from "./parse_sql_2.js";

import { Driver, BinMsg } from "./helpers.ts";

const driver: Driver = stroppy;

declare function defineConfig(config: BinMsg<GlobalConfig>): void;

if (typeof globalThis.defineConfig !== "function") {
  globalThis.defineConfig = driver.defineConfigBin;
}

declare const __ENV: Record<string, string | undefined>;
declare const __SQL_FILE: string;

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
defineConfig(
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
      version: "",
      runId: "",
      seed: "0",
      metadata: {},
    }),
  ),
);

declare function open(path: string): string; // k6 function to get files content

const content: string = open(__SQL_FILE);
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
