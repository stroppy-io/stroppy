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
import { GlobalConfig, Status, Generation_Rule } from "./stroppy.pb.js";

// Sql Driver interface
// is an interface of stroppy go module
interface Driver {
  runQuery(sql: string, args: Record<string, any>): void; // TODO: return value, is it posible to make it generic?
}
interface Generator {
  next(): any;
}
declare function NewDriverByConfig(configBin: Uint8Array): Driver;
declare function NotifyStep(name: String, status: Number): void;
declare function Teardown(): Error;
declare function NewGeneratorByRuleBin(
  seed: Number,
  rule: Uint8Array,
): Generator;

declare const __ENV: Record<string, string | undefined>;
declare const __SQL_FILE: string;

function NewGeneratorByRule(seed: Number, rule: Generation_Rule): Generator {
  return NewGeneratorByRuleBin(seed, Generation_Rule.toBinary(rule));
}

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

const driver = NewDriverByConfig(
  GlobalConfig.toBinary(
    GlobalConfig.create({
      runId: "",
      seed: "0",
      version: "",
      metadata: {},
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

export function setup() {
  NotifyStep("create_schema", Status.STATUS_RUNNING);
  NotifyStep("create_schema", Status.STATUS_COMPLETED);
  NotifyStep("load_data", Status.STATUS_RUNNING);
  NotifyStep("load_data", Status.STATUS_COMPLETED);
  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}
const gen = NewGeneratorByRule(
  0,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { min: 0, max: 100 } },
  }),
);
console.log("gen is", gen);

export function workload() {
  const value = gen.next();
  console.log("value is", value);
  driver.runQuery("select 1;", {});
  driver.runQuery("select 90000 + :value + :second;", {
    value,
    second: gen.next(),
  });

  driver.runQuery("select :a::int + :b::int", { a: 34, b: 35 });
  driver.runQuery("select 'Hello, ' || :a || '!'", { a: "world" });
}

export function teardown() {
  NotifyStep("workload", Status.STATUS_COMPLETED);
  Teardown();
}
