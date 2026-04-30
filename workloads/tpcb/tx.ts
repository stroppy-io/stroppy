import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, Step, ENV, TxIsolationName, declareDriverSetup } from "./helpers.ts";
import {
  Alphabet,
  Attr,
  Draw,
  DrawRT,
  Expr,
  InsertMethod as DatagenInsertMethod,
  Rel,
  std,
} from "./datagen.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

declare const __VU: number;

// TPC-B Configuration Constants
const SCALE_FACTOR = ENV(["SCALE_FACTOR", "BRANCHES"], 1, "TPC-B scale factor");
const POOL_SIZE   = ENV("POOL_SIZE", 50, "Connection pool size");
const LOAD_WORKERS = ENV("LOAD_WORKERS", 0, "Load-time worker count per spec (0 = framework default)") as number;

const BRANCHES = SCALE_FACTOR;
const TELLERS  = 10 * SCALE_FACTOR;
const ACCOUNTS = 100_000 * SCALE_FACTOR;

// TPC-B canonical fan-out: 10 tellers per branch, 100_000 accounts per branch.
const TELLERS_PER_BRANCH  = 10;
const ACCOUNTS_PER_BRANCH = 100_000;

// Filler widths (TPC-B §1.3.2 Table 1).
const BRANCH_FILLER_LEN  = 88;
const TELLER_FILLER_LEN  = 84;
const ACCOUNT_FILLER_LEN = 84;

// Spec-frozen per-population seeds. Chosen once, fixed for reproducibility.
const SEED_BRANCHES = 0x7B01B;
const SEED_TELLERS  = 0x7E11E;
const SEED_ACCOUNTS = 0xACC07;

// K6 options — VUs/duration set via CLI or k6 defaults.
export const options: Options = {
  setupTimeout: String(SCALE_FACTOR) + "m",
};

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql/pico/ydb)
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "native",
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

// Right-pad a literal string with spaces to exactly `width` bytes, then use
// the result as the constant filler payload. Matches the CHAR(n) wire format
// pgbench writes during initialization.
function fillerAscii(width: number): ReturnType<typeof Draw.ascii> {
  const len = Expr.lit(width);
  return Draw.ascii({ min: len, max: len, alphabet: Alphabet.en });
}

// InsertSpec builders. Each derives its bid column arithmetically from the
// row index so the branch fan-out matches the TPC-B spec exactly.

function branchesSpec() {
  return Rel.table("pgbench_branches", {
    size: BRANCHES,
    seed: SEED_BRANCHES,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      bid:      Attr.rowId(),
      bbalance: Expr.lit(0),
      filler:   fillerAscii(BRANCH_FILLER_LEN),
    },
  });
}

function tellersSpec() {
  // tid: 1..TELLERS; bid: (tid-1)/10 + 1 = rowIndex()/10 + 1
  return Rel.table("pgbench_tellers", {
    size: TELLERS,
    seed: SEED_TELLERS,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      tid: Attr.rowId(),
      bid: Expr.add(
        Expr.div(Attr.rowIndex(), Expr.lit(TELLERS_PER_BRANCH)),
        Expr.lit(1),
      ),
      tbalance: Expr.lit(0),
      filler:   fillerAscii(TELLER_FILLER_LEN),
    },
  });
}

function accountsSpec() {
  // aid: 1..ACCOUNTS; bid: (aid-1)/100000 + 1 = rowIndex()/100000 + 1
  return Rel.table("pgbench_accounts", {
    size: ACCOUNTS,
    seed: SEED_ACCOUNTS,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      aid: Attr.rowId(),
      bid: Expr.add(
        Expr.div(Attr.rowIndex(), Expr.lit(ACCOUNTS_PER_BRANCH)),
        Expr.lit(1),
      ),
      abalance: Expr.lit(0),
      filler:   fillerAscii(ACCOUNT_FILLER_LEN),
    },
  });
}

// Setup function: drop, create schema, load data (no procedures in tx variant)
export function setup() {
  Step("drop_schema", () => {
    sql("drop_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_schema", () => {
    sql("create_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("load_data", () => {
    driver.insertSpec(branchesSpec());
    driver.insertSpec(tellersSpec());
    driver.insertSpec(accountsSpec());
  });

  Step.begin("workload");
  return;
}

// Per-VU seed for tx-time draws. Each slot name hashes to a distinct
// offset so concurrent VUs draw independent sequences. __VU is 1-based
// in k6; the probe pass (script metadata extraction) runs outside k6
// so we guard with typeof to avoid a ReferenceError there.
const _vuId = typeof __VU === "number" ? __VU : 0;
const seedOf = (slot: string): number => {
  let h = 0;
  for (let i = 0; i < slot.length; i++) h = (h * 131 + slot.charCodeAt(i)) | 0;
  return ((_vuId | 0) * 0x9e3779b9) ^ (h >>> 0);
};

// Generators for transaction parameters (per-VU runtime state; tx-level SQL
// unchanged from the pre-datagen workload). Built at init scope because
// DrawRT constructors resolve the xk6 stroppy module via require(), which
// k6 only permits during init.
const aidGen   = DrawRT.intUniform(seedOf("aid"),   1, ACCOUNTS);
const tidGen   = DrawRT.intUniform(seedOf("tid"),   1, TELLERS);
const bidGen   = DrawRT.intUniform(seedOf("bid"),   1, BRANCHES);
const deltaGen = DrawRT.intUniform(seedOf("delta"), -5000, 5000);

// Per-VU monotonic counter for history PK (uniform across all dialects).
let hcounter = (typeof __VU === "number" ? __VU : 1) * 1_000_000_000;
const nextHid = () => ++hcounter;

// Silence unused-import warning for std — the stdlib namespace is part of
// the public datagen surface and kept imported so future tx.ts tweaks
// (e.g. std.format for dynamic filler) don't need to restructure imports.
void std;

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

  driver.beginTx({ isolation: TX_ISOLATION, name: "tpcb" }, (tx) => {
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
