import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";

import { DriverX, Step, declareDriverSetup } from "./helpers.ts";
import {
  Alphabet,
  Attr,
  Draw,
  DrawRT,
  Expr,
  InsertMethod as DatagenInsertMethod,
  Rel,
} from "./datagen.ts";

// simple.ts — minimal stroppy demo for new users. Loads a small table
// via driver.insertSpec, runs one query, asserts the row count, and
// tears down. No stored procs, no multi-dialect SQL, no mix weights.
// Intended as the first workload a new user reads.
//
// Run against the built-in postgres preset:
//   stroppy run simple -D url=postgres://user:pw@localhost:5432/postgres
// Or against any driver via --driver:
//   stroppy run simple -d noop

export const options: Options = {
  setupTimeout: "1m",
  scenarios: {
    workload: { executor: "shared-iterations", exec: "workload", vus: 1, iterations: 1 },
  },
};

const driverConfig = declareDriverSetup(0, {
  url:        "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
});
const driver = DriverX.create().setup(driverConfig);

const DEMO_ROWS = 100;
const DEMO_SEED = 0xC0FFEE;

// A three-column demo table. id is the 1-based row counter, label is
// an 8-char ASCII string, value is a uniformly-drawn integer in [0, 999].
function demoSpec() {
  return Rel.table("stroppy_demo", {
    size: DEMO_ROWS,
    seed: DEMO_SEED,
    method: DatagenInsertMethod.PLAIN_BULK,
    attrs: {
      id:    Attr.rowId(),
      label: Draw.ascii({ min: Expr.lit(8), max: Expr.lit(8), alphabet: Alphabet.en }),
      value: Draw.intUniform({ min: Expr.lit(0), max: Expr.lit(999) }),
    },
  });
}

export function setup() {
  Step("drop_schema", () => {
    driver.exec("DROP TABLE IF EXISTS stroppy_demo");
  });
  Step("create_schema", () => {
    driver.exec("CREATE TABLE stroppy_demo (id INT PRIMARY KEY, label TEXT, value INT)");
  });
  Step("load_data", () => {
    driver.insertSpec(demoSpec());
  });
  Step.begin("workload");
}

// A handful of DrawRT samples used inside the workload loop. These are
// built at init scope because DrawRT's backing module resolves
// lazily via k6 require(), which is only legal during init.
const pickIdGen = DrawRT.intUniform(DEMO_SEED ^ 1, 1, DEMO_ROWS);

export function workload() {
  // 1. Aggregate check: the loaded row count equals DEMO_ROWS.
  const count = Number(driver.queryValue("SELECT COUNT(*) FROM stroppy_demo"));
  if (count !== DEMO_ROWS) {
    throw new Error(`expected ${DEMO_ROWS} rows, got ${count}`);
  }
  console.log(`loaded ${count} rows into stroppy_demo`);

  // 2. Per-row lookup: pick 3 ids via a tx-time DrawRT generator and
  //    confirm each row is present. Shows how tx-time randomness is
  //    wired — construct the Drawer at init, call .next() in the
  //    workload body.
  for (let i = 0; i < 3; i++) {
    const id = Number(pickIdGen.next());
    const label = driver.queryValue("SELECT label FROM stroppy_demo WHERE id = :id", { id });
    console.log(`id=${id} → label=${label}`);
  }
}

export function teardown() {
  Step.end("workload");
  driver.exec("DROP TABLE IF EXISTS stroppy_demo");
  Teardown();
}
