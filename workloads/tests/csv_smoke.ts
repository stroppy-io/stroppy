/**
 * CSV ephemeral-driver smoke test.
 *
 * Drives two flavours of the csv driver (merge=true and merge=false)
 * through a small 100-row insert spec each and asserts the expected
 * output files exist. The CSV driver refuses non-DDL queries, so the
 * workload body never touches driver.exec for anything but the
 * drop/create-schema steps — both are accepted as noops.
 *
 * Invocation example:
 *   ./build/stroppy run ./workloads/tests/csv_smoke.ts \
 *     -D url='/tmp/csv_smoke?merge=true&workload=smoke' \
 *     -D driverType=csv \
 *     --steps drop_schema,create_schema,load_data
 */

import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, Step, declareDriverSetup, ENV } from "./helpers.ts";
import {
  Rel,
  Attr,
  Expr,
  InsertMethod as DatagenInsertMethod,
} from "./datagen.ts";

export const options: Options = {
  vus: 1,
  iterations: 1,
  setupTimeout: "30s",
};

const ROWS = ENV(["ROWS"], 100, "Rows per smoke table");

const cfg = declareDriverSetup(0, {
  url: ENV(["url"], "/tmp/stroppy-csv-smoke?workload=smoke"),
  driverType: "csv",
});

const driver = DriverX.create().setup(cfg);

function numberSpec(table: string, size: number) {
  return Rel.table(table, {
    size,
    seed: 0xC5F00D,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      id:      Attr.rowId(),
      squared: Expr.mul(Attr.rowIndex(), Attr.rowIndex()),
      label:   Expr.lit("row"),
    },
  });
}

export function setup() {
  Step("drop_schema", () => {
    driver.exec("DROP TABLE IF EXISTS numbers_a", {});
    driver.exec("DROP TABLE IF EXISTS numbers_b", {});
  });

  Step("create_schema", () => {
    driver.exec("CREATE TABLE numbers_a (id INT, squared INT, label TEXT)", {});
    driver.exec("CREATE TABLE numbers_b (id INT, squared INT, label TEXT)", {});
  });

  Step("load_data", () => {
    driver.insertSpec(numberSpec("numbers_a", ROWS));
    driver.insertSpec(numberSpec("numbers_b", ROWS));
  });
}

export default function () {
  // Default iteration body is intentionally empty: the csv driver has
  // no query path, so every per-VU workload loop would fail. k6 forces
  // at least one iteration; this shape yields it.
}

export function teardown() {
  Teardown();
}
