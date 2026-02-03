import { Options } from "k6/options";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import { NotifyStep, Teardown } from "k6/x/stroppy";

import { DriverConfig_DriverType, Status } from "./stroppy.pb.js";
import { DriverX, NewGen, NewGroupGen, AB, R, S } from "./helpers.ts";

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

const driver: DriverX = DriverX.fromConfig({
  runId: "",
  seed: "0",
  version: "",
  metadata: {},
  driver: {
    url: __ENV.DRIVER_URL || "postgres://postgres:postgres@localhost:5432",
    driverType: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
    dbSpecific: {
      fields: [],
    },
  },
});

export function setup() {
  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}

// Raw generator defenition with Generation_Rule
const gen = NewGen(0, {
  kind: { oneofKind: "int32Range", int32Range: { min: 0, max: 100 } },
});

// The generator of strings of length = 10, made using the English alphabet
const gen2 = NewGen(1, R.str(10, AB.en));

// Group of generators, run and check logs to find out the pattern
const groupGen = NewGroupGen(2, {
  params: R.params({
    some: S.int32(1, 2),
    second: S.int32(1, 3),
    bool: R.bool(1, true),
  }),
});

export function workload() {
  const value = gen.next();
  console.log("value is", value);

  // driver can run query
  driver.runQuery("select 1;", {});

  // and it uses :arg syntax to get arguments
  driver.runQuery("select 90000 + :value + :second;", {
    value,
    second: gen.next(),
  });

  driver.runQuery("select :a::int + :b::int", { a: 34, b: 35 });
  driver.runQuery("select 'Hello, ' || :a || '!'", { a: gen2.next() });

  for (let i = 0; i < 12; i++) {
    const [a, b, c] = groupGen.next();
    console.log("a", a, "b", b, "c", c);
  }
}

export function teardown() {
  NotifyStep("workload", Status.STATUS_COMPLETED);
  Teardown();
}
