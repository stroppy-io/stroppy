import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;
import stroppy from "k6/x/stroppy";

import { Options } from "k6/options";
import {
  UnitDescriptor,
  DriverTransactionStat,
  DriverConfig,
  GlobalConfig,
  WorkloadDescriptor,
  InsertDescriptor,
  Status,
} from "./stroppy.pb.js";

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
  defineConfig(config: DriverConfig): void;
  defineConfigBin(config: DriverConfig): void;
}

const driver: Driver = stroppy;

function defineConfig(config: DriverConfig): void {
  driver.defineConfig(config);
}

// Initialize driver with GlobalConfig
defineConfig({
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

export function setup() {
  driver.notifyStep("create_schema", Status.STATUS_RUNNING);
  driver.notifyStep("create_schema", Status.STATUS_COMPLETED);
  driver.notifyStep("load_data", Status.STATUS_RUNNING);
  driver.notifyStep("load_data", Status.STATUS_COMPLETED);
  driver.notifyStep("workload", Status.STATUS_RUNNING);
  return;
}

export function workload() {
  driver.runQuery("select 1;", {});
  driver.runQuery("select :fuck;", { fuck: 96 });
  driver.runQuery("select 13;", {});
}

export function teardown() {
  driver.notifyStep("workload", Status.STATUS_COMPLETED);
  driver.teardown();
}
