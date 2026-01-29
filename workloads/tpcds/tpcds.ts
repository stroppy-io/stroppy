import { Options } from "k6/options";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import { NotifyStep, Teardown } from "k6/x/stroppy";

import { DriverConfig_DriverType, Status } from "./stroppy.pb.js";
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
    driverType: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
    dbSpecific: {
      fields: [],
    },
  },
  version: "",
  runId: "",
  seed: "0",
  metadata: {},
});

const parsedQueries = parse_sql(open(__SQL_FILE));

export function setup() {
  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}

export function workload() {
  parsedQueries.forEach((query) => {
    console.log(`tpc-ds-like: ${query.name}`);
    driver.runQuery(query.sql, {});
  });
}

export function teardown() {
  NotifyStep("workload", Status.STATUS_COMPLETED);
  Teardown();
}
