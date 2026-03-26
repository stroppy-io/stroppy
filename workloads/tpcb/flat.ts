import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, AB, C, R, Step, S, ENV, declareDriverSetup } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file path (defaults to ./ansi.sql)")
  || "./ansi.sql";

// TPC-B Configuration Constants
const SCALE_FACTOR = ENV(["SCALE_FACTOR", "BRANCHES"], 1, "TPC-B scale factor");
const BRANCHES = SCALE_FACTOR;
const TELLERS = 10 * SCALE_FACTOR;
const ACCOUNTS = 100000 * SCALE_FACTOR;

// K6 options
export const options: Options = {
  setupTimeout: String(SCALE_FACTOR) + "m",
};

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql/pico)
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "copy_from",
});

const driver = DriverX.create().setup(driverConfig);

const sql = parse_sql_with_sections(open(SQL_FILE));

export function setup() {
  Step("cleanup", () => {
    sql("cleanup").forEach((query) => driver.exec(query, {}));
  });

  Step("create_schema", () => {
    sql("create_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("load_data", () => {
    driver.insert("pgbench_branches", BRANCHES, {
      params: {
        bid: S.int32(1, BRANCHES),
        bbalance: C.int32(0),
        filler: R.str(88, AB.en),
      },
    });

    driver.insert("pgbench_tellers", TELLERS, {
      params: {
        tid: S.int32(1, TELLERS),
        bid: R.int32(1, BRANCHES),
        tbalance: C.int32(0),
        filler: R.str(84, AB.en),
      },
    });

    driver.insert("pgbench_accounts", ACCOUNTS, {
      params: {
        aid: S.int32(1, ACCOUNTS),
        bid: R.int32(1, BRANCHES),
        abalance: C.int32(0),
        filler: R.str(84, AB.en),
      },
    });
  });

  Step.begin("workload");
  return;
}

// Generators for transaction parameters
const aidGen = R.int32(1, ACCOUNTS).gen();
const tidGen = R.int32(1, TELLERS).gen();
const bidGen = R.int32(1, BRANCHES).gen();
const deltaGen = R.int32(-5000, 5000).gen();

// TPC-B transaction workload (flat — no stored procedures, uses explicit tx)
export default function (): void {
  const aid = aidGen.next();
  const tid = tidGen.next();
  const bid = bidGen.next();
  const delta = deltaGen.next();

  driver.beginTx((tx) => {
    tx.exec(sql("workload", "update_account")!, { aid, delta });
    tx.exec(sql("workload", "get_balance")!, { aid });
    tx.exec(sql("workload", "update_teller")!, { tid, delta });
    tx.exec(sql("workload", "update_branch")!, { bid, delta });
    tx.exec(sql("workload", "insert_history")!, { tid, bid, aid, delta });
  });
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
