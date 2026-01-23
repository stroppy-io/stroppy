import { Options } from "k6/options";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import { NewDriverByConfig, NotifyStep, Teardown } from "k6/x/stroppy";

import {
  GlobalConfig,
  Status,
  Generation_Rule,
  QueryParamGroup,
} from "./stroppy.pb.js";
import { NewGeneratorByRule, NewGroupGeneratorByRules } from "./helpers.ts";

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

const groupGen = NewGroupGeneratorByRules(
  0,
  QueryParamGroup.create({
    name: "Some",
    params: [
      {
        generationRule: Generation_Rule.create({
          kind: { oneofKind: "int32Range", int32Range: { min: 1, max: 2 } },
          unique: true,
        }),
      },
      {
        generationRule: Generation_Rule.create({
          kind: { oneofKind: "int32Range", int32Range: { min: 1, max: 3 } },
          unique: true,
        }),
      },
      {
        generationRule: Generation_Rule.create({
          kind: { oneofKind: "boolRange", boolRange: { ratio: 1 } },
          unique: true,
        }),
      },
    ],
  }),
);

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

  for (let i = 0; i < 12; i++) {
    const [a, b, c] = groupGen.next();
    console.log("a", a, "b", b, "c", c);
  }
}

export function teardown() {
  NotifyStep("workload", Status.STATUS_COMPLETED);
  Teardown();
}
