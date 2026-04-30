import { sleep } from "k6";
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

const ROWS = ENV(["ROWS"], 100, "Rows per metrics smoke table");

const cfg = declareDriverSetup(0, {
  url: ENV(["url"], "noop://metrics"),
  driverType: "noop",
  defaultTxIsolation: "none",
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
  Step("load_data", () => {
    driver.insertSpec(numberSpec("numbers_a", ROWS));
    driver.insertSpec(numberSpec("numbers_b", ROWS));
  });
  Step.begin("workload");
}

export default function () {
  driver.exec({
    sql: "SELECT 1",
    tags: { name: "outside_query", type: "metrics" },
  }, {});

  driver.beginTx({ isolation: "none", name: "metrics_tx" }, (tx) => {
    tx.exec({
      sql: "SELECT 1",
      tags: { name: "inside_tx", type: "metrics" },
    }, {});
  });

  // The Go-side qps/tps metrics are sampled once per second.
  sleep(1.2);
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
