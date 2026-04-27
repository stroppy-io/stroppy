import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, Step, ENV, declareDriverSetup } from "./helpers.ts";
import {
  Alphabet,
  Attr,
  Draw,
  DrawRT,
  Expr,
  InsertMethod as DatagenInsertMethod,
  Rel,
} from "./datagen.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

declare const __VU: number;

// TPC-B Configuration Constants
const SCALE_FACTOR = ENV(["SCALE_FACTOR", "BRANCHES"], 1, "TPC-B scale factor");
const POOL_SIZE   = ENV("POOL_SIZE", 50, "Connection pool size");

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

// Spec-frozen per-population seeds. Shared with tx.ts so a procs run
// produces identical load data at the same SCALE_FACTOR.
const SEED_BRANCHES = 0x7B01B;
const SEED_TELLERS  = 0x7E11E;
const SEED_ACCOUNTS = 0xACC07;

// K6 options — VUs/duration set via CLI or k6 defaults.
export const options: Options = {
  setupTimeout: String(SCALE_FACTOR) + "m",
};

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql)
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "native",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

// procs.ts targets pg + mysql only — picodata and ydb have no stored procedures.
if (driverConfig.driverType === "picodata" || driverConfig.driverType === "ydb") {
  throw new Error(
    `tpcb/procs.ts only supports postgres and mysql (got driverType=${driverConfig.driverType}). ` +
    `Use tpcb/tx.ts for picodata/ydb.`,
  );
}

const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql:    "./mysql.sql",
};
const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file path (defaults per driverType)")
  ?? _sqlByDriver[driverConfig.driverType!]
  ?? "./pg.sql";

const driver = DriverX.create().setup(driverConfig);

const sql = parse_sql_with_sections(open(SQL_FILE));

// Right-pad a literal string with spaces to exactly `width` bytes, then use
// the result as the constant filler payload. Matches the CHAR(n) wire format
// pgbench writes during initialization.
function fillerAscii(width: number): ReturnType<typeof Draw.ascii> {
  const len = Expr.lit(width);
  return Draw.ascii({ min: len, max: len, alphabet: Alphabet.en });
}

// InsertSpec builders — structurally identical to tx.ts so both
// workloads share a load schema under the same seeds.

function branchesSpec() {
  return Rel.table("pgbench_branches", {
    size: BRANCHES,
    seed: SEED_BRANCHES,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      bid:      Attr.rowId(),
      bbalance: Expr.lit(0),
      filler:   fillerAscii(BRANCH_FILLER_LEN),
    },
  });
}

function tellersSpec() {
  return Rel.table("pgbench_tellers", {
    size: TELLERS,
    seed: SEED_TELLERS,
    method: DatagenInsertMethod.NATIVE,
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
  return Rel.table("pgbench_accounts", {
    size: ACCOUNTS,
    seed: SEED_ACCOUNTS,
    method: DatagenInsertMethod.NATIVE,
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

// Setup function: drop, create schema + procs, load data
export function setup() {
  Step("drop_schema", () => {
    sql("drop_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_schema", () => {
    sql("create_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_procedures", () => {
    sql("create_procedures").forEach((query) => driver.exec(query, {}));
  });

  Step("load_data", () => {
    driver.insertSpec(branchesSpec());
    driver.insertSpec(tellersSpec());
    driver.insertSpec(accountsSpec());
  });

  Step.begin("workload");
  return;
}

// Per-VU seed for tx-time draws. Mirrors the tx.ts formula so procs
// and tx runs at the same __VU produce identical draw sequences.
const _vuId = typeof __VU === "number" ? __VU : 0;
const seedOf = (slot: string): number => {
  let h = 0;
  for (let i = 0; i < slot.length; i++) h = (h * 131 + slot.charCodeAt(i)) | 0;
  return ((_vuId | 0) * 0x9e3779b9) ^ (h >>> 0);
};

// Generators for transaction parameters (per-VU runtime state).
const aidGen   = DrawRT.intUniform(seedOf("aid"),   1, ACCOUNTS);
const tidGen   = DrawRT.intUniform(seedOf("tid"),   1, TELLERS);
const bidGen   = DrawRT.intUniform(seedOf("bid"),   1, BRANCHES);
const deltaGen = DrawRT.intUniform(seedOf("delta"), -5000, 5000);

// Per-VU monotonic counter for history PK (uniform across all dialects).
let hcounter = (typeof __VU === "number" ? __VU : 1) * 1_000_000_000;
const nextHid = () => ++hcounter;

// TPC-B transaction workload — single stored proc call per iteration.
export default function (): void {
  driver.exec(sql("workload_procs", "tpcb_transaction")!, {
    p_aid: aidGen.next(),
    p_tid: tidGen.next(),
    p_bid: bidGen.next(),
    p_delta: deltaGen.next(),
    p_hid: nextHid(),
  });
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
