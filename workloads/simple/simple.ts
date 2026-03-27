import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, AB, R, S, Step, setSeed, ENV, declareDriverSetup } from "./helpers.ts";

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

const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
});

const driver = DriverX.create().setup(driverConfig);

setSeed(42);

export function setup() {
  Step("example", () => {
    // You can structure test into steps with Step function.
  })
  // Also you can use Step.begin and Step.end functions to define step.
  Step.begin("workload");
  return;
}

// No seed → uses module-wide default (0 if not set) → random each run.
const genRandom = R.int32(0, 100).gen();

// Explicit seed → always produces the same sequence regardless of global seed.
const genFixed = R.str(10, AB.en).gen(111);

// Sequence generator: produces 1, 2, 3, ... exhausting after max.
const seqGen = S.int32(1, 10).gen();

// Group generator: cartesian-product of dependent params.
// Useful for composite keys — see logs for the pattern.
const groupGen = R.group({
    some: S.int32(1, 2),
    second: S.int32(1, 3),
    bool: R.bool(1, true),
  }).gen(5)

export function workload() {
  // driver uses :arg syntax for query parameters
  driver.exec("select 1;", {});

  const value = genRandom.next();
  console.log("random value:", value);
  driver.exec("select 90000 + :value + :second;", {
    value,
    second: genRandom.next(),
  });

  console.log("value is:",
    driver.queryValue("select :a::int + :b::int", { a: 34, b: 35 }));

  const str = genFixed.next();
  console.log("fixed-seed string (same every run):", str);
  driver.exec("select 'Hello, ' || :a || '!'", { a: str });


  console.log("sequence (exhausts after 10):", seqGen.next());

  for (let i = 0; i < 12; i++) {
    const [a, b, c] = groupGen.next();
    console.log("group cartesian product — a:", a, "b:", b, "c:", c);
  }
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
