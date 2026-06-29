// Shared TPC-B driver core for both execution variants (procs.ts: a single
// server-side stored-procedure call; tx.ts: the canonical 5-statement
// client-side transaction). Everything that does not depend on how the
// transaction is issued lives here: configuration, the seeded load specs, the
// prepare/measure lifecycle, the scenario, and teardown.
import { Teardown } from "k6/x/stroppy";
import {
  DriverX,
  Step,
  ENV,
  GlobalOnce,
  TxIsolationName,
  declareDriverSetup,
  declareScenario,
} from "./helpers.ts";
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
export const SCALE_FACTOR = ENV(["SCALE_FACTOR", "BRANCHES"], 1, "TPC-B scale factor");
const POOL_SIZE = ENV("POOL_SIZE", 50, "Connection pool size");
const LOAD_WORKERS = ENV("LOAD_WORKERS", 0, "Load-time worker count per spec (0 = framework default)") as number;
// PostgreSQL only: create LOGGED, flip to UNLOGGED for a WAL-free bulk load,
// then back to LOGGED before the workload. Disable with PG_UNLOGGED=false.
const PG_UNLOGGED = ENV("PG_UNLOGGED", "true", "pg only: bulk-load with UNLOGGED tables, flip back to LOGGED after") !== "false";

const BRANCHES = SCALE_FACTOR;
const TELLERS = 10 * SCALE_FACTOR;
const ACCOUNTS = 100_000 * SCALE_FACTOR;

// TPC-B canonical fan-out: 10 tellers per branch, 100_000 accounts per branch.
const TELLERS_PER_BRANCH = 10;
const ACCOUNTS_PER_BRANCH = 100_000;

// Filler widths (TPC-B §1.3.2 Table 1).
const BRANCH_FILLER_LEN = 88;
const TELLER_FILLER_LEN = 84;
const ACCOUNT_FILLER_LEN = 84;

// Spec-frozen per-population seeds. Shared by both variants so a procs run and
// a tx run produce identical load data at the same SCALE_FACTOR.
const SEED_BRANCHES = 0x7B01B;
const SEED_TELLERS = 0x7E11E;
const SEED_ACCOUNTS = 0xACC07;

// One scenario, two shapes: throughput (constant-vus, set DURATION) vs power
// (shared-iterations). Tune via VUS/DURATION/ITER/MAX_DURATION env.
export const options = {
  scenarios: declareScenario("tpcb"),
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
};

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql/pico/ydb)
export const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "native",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

export const driverType = driverConfig.driverType;

const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql: "./mysql.sql",
  picodata: "./pico.sql",
  ydb: "./ydb.sql",
};
const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file path (defaults per driverType)")
  ?? _sqlByDriver[driverType!]
  ?? "./pg.sql";

// Per-driver isolation default. picodata MUST be "none" — picodata.Begin always errors.
const _isoByDriver: Record<string, TxIsolationName> = {
  postgres: "read_committed",
  mysql: "read_committed",
  picodata: "none",
  ydb: "serializable",
};
export const TX_ISOLATION = (
  ENV("TX_ISOLATION", ENV.auto, "Override transaction isolation level (read_committed/serializable/conn/none/...)")
  ?? _isoByDriver[driverType!]
  ?? "read_committed"
) as TxIsolationName;

export const driver = DriverX.create().setup(driverConfig);

const sql = parse_sql_with_sections(open(SQL_FILE));
export { sql };

// Run an SQL section by name; a missing section (e.g. create_indexes on a
// dialect that ships none) is a no-op rather than an error.
function runSection(name: string): void {
  (sql(name) ?? []).forEach((query) => driver.exec(query, {}));
}

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
      bid: Attr.rowId(),
      bbalance: Expr.lit(0),
      filler: fillerAscii(BRANCH_FILLER_LEN),
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
      bid: Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(TELLERS_PER_BRANCH)), Expr.lit(1)),
      tbalance: Expr.lit(0),
      filler: fillerAscii(TELLER_FILLER_LEN),
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
      bid: Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(ACCOUNTS_PER_BRANCH)), Expr.lit(1)),
      abalance: Expr.lit(0),
      filler: fillerAscii(ACCOUNT_FILLER_LEN),
    },
  });
}

// Whether the UNLOGGED bulk-load dance applies: pg only, and not disabled.
const useUnlogged = PG_UNLOGGED && driverType === "postgres";

// Full prepare lifecycle as individually-named, skippable steps. Index build
// and ANALYZE run AFTER the bulk load (one-shot is far cheaper than
// per-row maintenance, and the planner needs fresh stats). `procedures` adds
// the stored-procedure DDL for the procs.ts variant.
//
// Each step is gated by the shared --steps/--no-steps machinery, so the
// canonical run shape is two passes: `--no-steps workload` (prep only) then
// `--steps workload` (measure against the already-loaded data).
function prepareDatabase(procedures: boolean): void {
  Step("drop_schema", () => runSection("drop_schema"));
  Step("create_schema", () => runSection("create_schema"));
  if (procedures) {
    Step("create_procedures", () => runSection("create_procedures"));
  }
  if (useUnlogged) {
    Step("set_unlogged", () => runSection("set_unlogged"));
  }
  Step("load_data", () => {
    driver.insertSpec(branchesSpec());
    driver.insertSpec(tellersSpec());
    driver.insertSpec(accountsSpec());
  });
  Step("create_indexes", () => runSection("create_indexes"));
  if (useUnlogged) {
    Step("set_logged", () => runSection("set_logged"));
  }
  // pgbench --foreign-keys semantics: FK constraints added post-load, once the
  // referenced rows exist, and backed by the bid indexes built above. Added AFTER
  // set_logged: pg checks FK persistence in BOTH directions, so a logged<->unlogged
  // FK edge may never exist -> creating them while tables are still UNLOGGED would
  // make the SET LOGGED flip fail. Absent on dialects without foreign keys
  // (ydb/picodata) -> no-op.
  Step("create_foreign_keys", () => runSection("create_foreign_keys"));
  Step("analyze", () => runSection("analyze"));
}

// Run the load exactly once across all VUs in the process. Concurrent VUs
// block here until the single loader finishes, then proceed to the workload.
export function prepare(procedures: boolean): void {
  GlobalOnce("tpcb.prepare", () => prepareDatabase(procedures));
}

// Per-VU seed for tx-time draws. Each slot name hashes to a distinct offset so
// concurrent VUs draw independent sequences. __VU is 1-based in k6; the probe
// pass runs outside k6 so we guard with typeof to avoid a ReferenceError.
const _vuId = typeof __VU === "number" ? __VU : 0;
const seedOf = (slot: string): number => {
  let h = 0;
  for (let i = 0; i < slot.length; i++) h = (h * 131 + slot.charCodeAt(i)) | 0;
  return ((_vuId | 0) * 0x9e3779b9) ^ (h >>> 0);
};

// Generators for transaction parameters (per-VU runtime state). Built at init
// scope because DrawRT constructors resolve the xk6 stroppy module via
// require(), which k6 only permits during init.
export const aidGen = DrawRT.intUniform(seedOf("aid"), 1, ACCOUNTS);
export const tidGen = DrawRT.intUniform(seedOf("tid"), 1, TELLERS);
export const bidGen = DrawRT.intUniform(seedOf("bid"), 1, BRANCHES);
export const deltaGen = DrawRT.intUniform(seedOf("delta"), -5000, 5000);

// Per-VU monotonic counter for history PK (uniform across all dialects).
let hcounter = (typeof __VU === "number" ? __VU : 1) * 1_000_000_000;
export const nextHid = (): number => ++hcounter;

export function teardown(): void {
  Teardown();
}
