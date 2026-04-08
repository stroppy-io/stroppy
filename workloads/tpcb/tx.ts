import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, AB, C, R, Step, S, ENV, TxIsolationName, declareDriverSetup } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

declare const __VU: number;

// TPC-B Configuration Constants
const SCALE_FACTOR = ENV(["SCALE_FACTOR", "BRANCHES"], 1, "TPC-B scale factor");
const DURATION    = ENV("DURATION", "1m", "Test duration");
const VUS_SCALE   = ENV("VUS_SCALE", 1, "VUs scale factor (multiplied with base 50)");
const POOL_SIZE   = ENV("POOL_SIZE", 50, "Connection pool size");

const BRANCHES = SCALE_FACTOR;
const TELLERS  = 10 * SCALE_FACTOR;
const ACCOUNTS = 100000 * SCALE_FACTOR;

// K6 options
export const options: Options = {
  setupTimeout: String(SCALE_FACTOR) + "m",
  scenarios: {
    tpcb: {
      executor: "constant-vus",
      vus: Math.max(1, Math.floor(50 * VUS_SCALE)),
      duration: DURATION,
    },
  },
};

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql/pico/ydb)
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "copy_from",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql:    "./mysql.sql",
  picodata: "./pico.sql",
  ydb:      "./ydb.sql",
};
const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file path (defaults per driverType)")
  ?? _sqlByDriver[driverConfig.driverType!]
  ?? "./pg.sql";

// Per-driver isolation default. picodata MUST be "none" — picodata.Begin always errors.
const _isoByDriver: Record<string, TxIsolationName> = {
  postgres: "read_committed",
  mysql:    "read_committed",
  picodata: "none",
  ydb:      "serializable",
};
const TX_ISOLATION = (
  ENV("TX_ISOLATION", ENV.auto, "Override transaction isolation level (read_committed/serializable/conn/none/...)")
  ?? _isoByDriver[driverConfig.driverType!]
  ?? "read_committed"
) as TxIsolationName;

const driver = DriverX.create().setup(driverConfig);

const sql = parse_sql_with_sections(open(SQL_FILE));

// Setup function: drop, create schema, load data (no procedures in tx variant)
export function setup() {
  Step("drop_schema", () => {
    sql("drop_schema").forEach((query) => driver.exec(query, {}));
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
const aidGen   = R.int32(1, ACCOUNTS).gen();
const tidGen   = R.int32(1, TELLERS).gen();
const bidGen   = R.int32(1, BRANCHES).gen();
const deltaGen = R.int32(-5000, 5000).gen();

// Per-VU monotonic counter for history PK (uniform across all dialects).
let hcounter = (typeof __VU === "number" ? __VU : 1) * 1_000_000_000;
const nextHid = () => ++hcounter;

// TPC-B transaction workload — explicit transaction matching pgbench's
// canonical 5-step script. The SELECT is a real round-trip: we pull abalance
// back via tx.queryValue so the read actually materializes client-side (that
// is what pgbench measures).
export default function (): void {
  const aid = aidGen.next();
  const tid = tidGen.next();
  const bid = bidGen.next();
  const delta = deltaGen.next();
  const hid = nextHid();

  driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
    tx.exec(sql("workload_tx_tpcb", "update_account")!, { aid, delta });

    const abalance = tx.queryValue<number>(
      sql("workload_tx_tpcb", "get_balance")!, { aid },
    );
    if (abalance === undefined) {
      throw new Error(`TPC-B: account ${aid} not found`);
    }

    tx.exec(sql("workload_tx_tpcb", "update_teller")!,  { tid, delta });
    tx.exec(sql("workload_tx_tpcb", "update_branch")!,  { bid, delta });
    tx.exec(sql("workload_tx_tpcb", "insert_history")!, { hid, tid, bid, aid, delta });
  });
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
