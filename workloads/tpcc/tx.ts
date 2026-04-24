import { Options } from "k6/options";
import { sleep } from "k6";
import { Teardown, NewPicker } from "k6/x/stroppy";
import { Counter, Trend, Step, DriverX, ENV, TxIsolationName, declareDriverSetup, retry, isSerializationError } from "./helpers.ts";
import {
  Alphabet,
  Attr,
  Dict,
  Draw,
  DrawRT,
  Expr,
  InsertMethod as DatagenInsertMethod,
  Rel,
  std,
} from "./datagen.ts";
import { C_LAST_DICT, tpccOriginalOr } from "./tpcc_helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

// ============================================================================
// Data-gen simplifications remaining after the Stage-E spec-parity pass.
// Transaction phase is byte-for-byte compliant; load phase follows TPC-C
// §4.3 except for the single deferred item below.
//
//   1. Per-order line count fixed at 10 (spec wants Uniform 5..15,
//      §4.3.3.1). Deferred: expressing a variable-degree child population
//      under Rel.table requires Relationship/Side composition; see plan
//      §16. With a fixed OL_CNT=10 the mean matches spec's midpoint and
//      sum(o_ol_cnt) == count(order_line) (CC4) still holds.
//   2. history is empty at load time per spec §4.3.4 (initial cardinality
//      0). Not a simplification — included here for completeness.
//
// Everything else in §4.3 is spec-compliant:
//   - c_last: 3-syllable cartesian from TPCC_SYLLABLES (C_LAST_DICT).
//   - c_credit: weighted 1:9 BC/GC via Expr.choose.
//   - i_data / s_data: "ORIGINAL" marker at random position in 10% rows.
//   - o_carrier_id: NULL for o_id > 2100 (last 900 per district), else
//     Uniform(1, 10). Uses Expr.if + Expr.litNull.
//   - o_c_id: std.permuteIndex keyed per (w_id, d_id) so each district
//     holds a distinct permutation of [1, 3000].
//   - ol_delivery_d: NULL for the undelivered tail (ol_o_id > 2100),
//     load-time timestamp for the delivered prefix.
//   - ol_amount: Uniform(0.01, 9999.99) for undelivered orders, 0.00
//     for delivered (per §4.3.3.1 column formula).
//   - c_since, o_entry_d: constant load-captured timestamp (§4.3.2.8).
// ============================================================================

// Post-run compliance counters for TPC-C auditing. See TPCC_COMPILANCE_REPORT.md
// §1.11 — these expose the observed rates of spec-mandated percentages so an
// operator can verify compliance without instrumenting the DB side.
const tpccNewOrderTotal       = new Counter("tpcc_new_order_total");
const tpccRollbackDecided     = new Counter("tpcc_rollback_decided");
const tpccRollbackDone        = new Counter("tpcc_rollback_done");
const tpccRemoteLineTotal     = new Counter("tpcc_remote_line_total");
const tpccRemoteLineRem       = new Counter("tpcc_remote_line_remote");
const tpccPaymentTotal        = new Counter("tpcc_payment_total");
const tpccPaymentRemote       = new Counter("tpcc_payment_remote");
// §1.6: 60% of Payment / Order-Status transactions must look up the
// customer by last name instead of by c_id. These counters expose the
// observed by-name rate so an operator can verify that the 60% roll
// actually reaches the DB (and wasn't lost to an early-return path).
const tpccPaymentByname       = new Counter("tpcc_payment_byname");
// §1.8: 10% of customers carry c_credit='BC' and Payment must compute
// the 500-char c_data log for them (c_id, c_d_id, c_w_id, d_id, w_id,
// h_amount, '|', old_c_data). This counter exposes the observed rate
// so an operator can verify it tracks the population ratio.
const tpccPaymentBc           = new Counter("tpcc_payment_bc");
const tpccOrderStatusTotal    = new Counter("tpcc_order_status_total");
const tpccOrderStatusByname   = new Counter("tpcc_order_status_byname");
const tpccDeliveryTotal       = new Counter("tpcc_delivery_total");
const tpccStockLevelTotal     = new Counter("tpcc_stock_level_total");
// T2.3: count serialization-failure retries. PG REPEATABLE READ uses
// snapshot isolation, so concurrent updates to the same row abort with
// SQLSTATE 40001. The retry() helper catches those, sleeps zero, and tries
// again — incrementing this counter each time so an operator can see how
// often retries are firing without grepping logs. Spec §5.2.5 still caps
// total tx_error_rate at 1%; with retries the un-retryable tail is what
// counts against that budget.
const tpccRetryAttempts       = new Counter("tpcc_retry_attempts");

// T3.2: per-transaction response-time Trends. Spec §5.2.5.4 sets 90p
// ceilings (NO/P/OS 5s, SL 20s, D 80s). The `true` second arg marks
// these as time trends so k6 formats values in ms/s and the threshold
// parser accepts "p(90)<5000" millisecond literals. Same metric names
// as procs.ts so post-run analysis is variant-agnostic.
const tpccNewOrderDuration    = new Trend("tpcc_new_order_duration", true);
const tpccPaymentDuration     = new Trend("tpcc_payment_duration", true);
const tpccOrderStatusDuration = new Trend("tpcc_order_status_duration", true);
const tpccDeliveryDuration    = new Trend("tpcc_delivery_duration", true);
const tpccStockLevelDuration  = new Trend("tpcc_stock_level_duration", true);

// TPC-C Configuration Constants
const POOL_SIZE   = ENV("POOL_SIZE", 100, "Connection pool size");
const WAREHOUSES  = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");
const LOAD_WORKERS = ENV("LOAD_WORKERS", 0, "Load-time worker count per spec (0 = framework default)") as number;
// T2.3: how many attempts the retry helper makes before giving up on a
// serialization failure. 3 = original try + 2 retries; immediate, no sleep.
// Override via -e RETRY_ATTEMPTS=N for benchmarking the isolation tradeoff.
const RETRY_ATTEMPTS = ENV("RETRY_ATTEMPTS", 3, "Max attempts for serialization-failure retries (1 = no retry)");

// TPC-C §5.2.5 pacing: keying time (constant, before tx) + think time
// (negative exponential, after tx) simulate real terminal operator behaviour.
// Disabled by default for raw throughput benchmarking; enable with
// -e PACING=true for spec-compliant pacing.
const PACING = ENV("PACING", "false", "Enable keying + think time delays (§5.2.5)") === "true";

// §5.2.5.2 minimum keying times (seconds), §5.2.5.7 Table.
const KEYING_TIME: Record<string, number> = {
  new_order: 18, payment: 3, order_status: 2, delivery: 2, stock_level: 2,
};
// §5.2.5.4 / §5.2.5.7: mean think time (seconds) for neg-exp distribution.
const THINK_TIME_MEAN: Record<string, number> = {
  new_order: 12, payment: 12, order_status: 10, delivery: 5, stock_level: 5,
};

// §5.2.5.4: T_t = -log(r) * μ, truncated at 10μ.
function thinkTime(txName: string): void {
  if (!PACING) return;
  const mu = THINK_TIME_MEAN[txName];
  let t = -Math.log(Math.random()) * mu;
  if (t > 10 * mu) t = 10 * mu;
  sleep(t);
}

function keyingTime(txName: string): void {
  if (!PACING) return;
  sleep(KEYING_TIME[txName]);
}

const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT  = 3000;
const ITEMS = 100000;

const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_STOCK     = WAREHOUSES * ITEMS;

declare const __VU: number;

// Per-VU seed for tx-time draws. Each slot name hashes to a distinct
// offset so concurrent VUs draw independent sequences. The VU guard
// matches the pattern used further down in the file (see `_vu` in the
// hid_counter block) — the probe VM runs without k6 and reports
// undefined, so we coerce that case to 0.
const seedOf = (slot: string): number => {
  let h = 0;
  for (let i = 0; i < slot.length; i++) h = (h * 131 + slot.charCodeAt(i)) | 0;
  const vu = (typeof __VU === "number" && __VU > 0) ? __VU : 0;
  return (vu * 0x9e3779b9) ^ (h >>> 0);
};

// Runtime NURand(255, 0, 999) picker used by the by-name branch of
// Payment and Order-Status (§2.5.1.2 / §2.6.1.2). Module-scoped so the
// NURand C constant is chosen once for the whole run — mirrors how the
// existing nurand1023 / nurand8191 pickers are scoped. Indexes into
// C_LAST_DICT (3-syllable cartesian, §4.3.2.3) populated by the load phase.
// cSalt=0 yields the spec-compliant deterministic-default C via
// splitmix64(0); pass per run-scope since the salt is constant for
// this process.
const nurand255Gen = DrawRT.nurand(seedOf("nurand255"), 255, 0, 999);

// K6 options — weighted dispatch inside default(), VUs/duration set via CLI or k6 defaults.
// T3.2: k6 thresholds on the per-tx Trend metrics auto-fail the run if any
// p90 breaches the spec §5.2.5.4 ceiling. Using the stock threshold syntax
// so k6 marks the run failed and the summary line shows a PASS/FAIL marker
// next to each metric — no manual assertion needed on top of handleSummary.
export const options: Options = {
  setupTimeout: String(WAREHOUSES * 5) + "m",
  // Include p99 in the per-trend percentiles k6 computes; default is
  // ["avg","min","med","max","p(90)","p(95)"] — adding p(99) so the
  // handleSummary breakdown shows the full distribution we advertise.
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  thresholds: {
    "tpcc_new_order_duration":    ["p(90)<5000"],
    "tpcc_payment_duration":      ["p(90)<5000"],
    "tpcc_order_status_duration": ["p(90)<5000"],
    "tpcc_stock_level_duration":  ["p(90)<20000"],
    "tpcc_delivery_duration":     ["p(90)<80000"],
  },
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

// Per-driver isolation default. TPC-C §3.4.0.1 Table 3-1 requires Level 3
// (phantom protection) for NO/P/D and Level 2 for OS. PG's REPEATABLE READ =
// snapshot isolation (phantom-protected); MySQL InnoDB's REPEATABLE READ uses
// next-key locking (phantom-protected). Both satisfy the spec.
// picodata MUST be "none" — picodata.Begin always errors.
// ydb default `serializable` is above spec and compliant.
const _isoByDriver: Record<string, TxIsolationName> = {
  postgres: "repeatable_read",
  mysql:    "repeatable_read",
  picodata: "none",
  ydb:      "serializable",
};
const TX_ISOLATION = (
  ENV("TX_ISOLATION", ENV.auto, "Override transaction isolation level (read_committed/serializable/conn/none/...)")
  ?? _isoByDriver[driverConfig.driverType!]
  ?? "read_committed"
) as TxIsolationName;

const driver = DriverX.create().setup(driverConfig);

// picodata/sbroad doesn't support SELECT ... OFFSET, so the by-name
// customer median pick (Payment §2.5.2.2, Order-Status §2.6.2.2) has to
// fetch all matching rows and index client-side. The other dialects keep
// the efficient `LIMIT 1 OFFSET :offset` SQL path.
const IS_PICODATA = driverConfig.driverType === "picodata";
// pg and ydb support UPDATE...RETURNING — merge UPDATE + SELECT into one
// round-trip in payment() for warehouse/district YTD updates.
const HAS_RETURNING = driverConfig.driverType === "postgres" || driverConfig.driverType === "ydb";

const sql = parse_sql_with_sections(open(SQL_FILE));

// Per-VU monotonic counter for h_id only. history has no natural PK in the
// TPC-C spec, but picodata/ydb require one, so we add h_id to all dialects
// and generate it client-side. o_id is NOT a counter — we read d_next_o_id
// from district at the start of each new_order tx (see below).
const _vu = (typeof __VU === "number" && __VU > 0) ? __VU : 1;
let hid_counter = _vu * 10_000_000;
const nextHid = (): number => ++hid_counter;

// Spec §5.2.2 / Clause 4.2: each VU ("terminal") is bound to a single home
// warehouse for the run. This is what drives the 1%/15% remote-access
// minimums in new_order/payment. Scaling beyond WAREHOUSES VUs wraps.
const HOME_W_ID = 1 + ((_vu - 1) % WAREHOUSES);

// Pick a uniformly-random OTHER warehouse in [1, WAREHOUSES] \ {HOME_W_ID}.
// Callers must guard with WAREHOUSES > 1; with a single warehouse there is
// no valid remote target and the caller must fall back to HOME_W_ID.
const _remoteWhGen = WAREHOUSES > 1
  ? DrawRT.intUniform(seedOf("remoteWh"), 1, WAREHOUSES - 1)
  : null;
function pickRemoteWh(): number {
  if (_remoteWhGen === null) return HOME_W_ID;
  const alt = _remoteWhGen.next() as number;
  return alt >= HOME_W_ID ? alt + 1 : alt;
}

// T2.3: thin wrapper that wires the module-wide retry budget and counter
// into every transaction body. Each retry counts ONCE in tpccRetryAttempts
// regardless of where in the body the abort fired. The wrapper preserves
// the spec §2.4.2.3 New-Order rollback sentinel — `isSerializationError`
// short-circuits on `tpcc_rollback:` so the rollback path always escapes
// the loop on the first attempt.
function tpccRetry<T>(fn: () => T): T {
  return retry(
    RETRY_ATTEMPTS,
    isSerializationError,
    fn,
    () => { tpccRetryAttempts.add(1); },
  );
}

// ============================================================================
// InsertSpec builders — nine TPC-C tables plus a 1000-entry lastname dict.
// Spec-derived row counts for WAREHOUSES=W:
//   warehouse  = W
//   district   = W × 10
//   customer   = W × 10 × 3000
//   item       = 100_000
//   stock      = W × 100_000
//   orders     = W × 10 × 3000
//   new_order  = W × 10 × 900   (orders 2101..3000 per district)
//   order_line = orders × 10    (fixed OL_CNT=10)
//   history    = 0              (empty at load)
// FK columns are derived from rowIndex() via integer arithmetic so the load
// phase composes into a single Rel.table per entity without nested Sides.
// ============================================================================

const ORDERS_DELIVERED   = 2100;
const ORDERS_UNDELIVERED = CUSTOMERS_PER_DISTRICT - ORDERS_DELIVERED; // 900
const OL_CNT_FIXED       = 10;
const ITEMS_PER_WH       = ITEMS;

// Per-population seeds — frozen once so a repeated run with the same
// WAREHOUSES produces a byte-identical load. Values are arbitrary
// 64-bit constants chosen only for mnemonic readability.
const SEED_WAREHOUSE  = 0xC0FFEE01;
const SEED_DISTRICT   = 0xC0FFEE02;
const SEED_CUSTOMER   = 0xC0FFEE03;
const SEED_ITEM       = 0xC0FFEE04;
const SEED_STOCK      = 0xC0FFEE05;
const SEED_ORDERS     = 0xC0FFEE06;
const SEED_ORDER_LINE = 0xC0FFEE07;
const SEED_NEW_ORDER  = 0xC0FFEE08;

// Currency literal note: `Expr.lit(300000.0)` collapses to int64 because
// `Number.isInteger(300000.0)` is true in JS, which trips YDB BulkUpsert
// on `Double` columns (w_ytd, d_ytd, c_credit_lim, c_balance,
// c_ytd_payment). `Expr.litFloat(...)` forces the Double oneof arm; other
// dialects accept an int64 into their DECIMAL/NUMERIC columns identically.

// Draw.ascii helper: fixed-width ASCII over an alphabet (default Alphabet.en).
function asciiFixed(
  width: number,
  alphabet: readonly { min: number; max: number }[] = Alphabet.en,
) {
  const n = Expr.lit(width);
  return Draw.ascii({ min: n, max: n, alphabet });
}

// Draw.ascii helper: variable-width ASCII over an alphabet.
function asciiRange(
  minLen: number,
  maxLen: number,
  alphabet: readonly { min: number; max: number }[] = Alphabet.en,
) {
  return Draw.ascii({ min: Expr.lit(minLen), max: Expr.lit(maxLen), alphabet });
}

// Spec §4.3.2.8 / §4.3.3.1: c_since, o_entry_d, and the delivered branch
// of ol_delivery_d all carry the OS-captured load-time timestamp. We
// snapshot it once at module load so every row in this run receives the
// same value — mirrors main's R.dateConst pattern and lets the compliance
// tests key off a single deterministic instant. `Expr.lit(Date)` emits
// int64 (epoch days on the wire), so we lift through `std.daysToDate`
// to get the time.Time scalar the driver layer expects on DATETIME /
// TIMESTAMP columns.
const LOAD_TIMESTAMP       = new Date();
const LOAD_TIMESTAMP_EXPR  = std.daysToDate(Expr.lit(LOAD_TIMESTAMP));

// Warehouse spec: w_id = rowIndex()+1 ∈ [1, WAREHOUSES].
function warehouseSpec() {
  return Rel.table("warehouse", {
    size: WAREHOUSES,
    seed: SEED_WAREHOUSE,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      w_id:       Attr.rowId(),
      w_name:     asciiRange(6, 10),
      w_street_1: asciiRange(10, 20),
      w_street_2: asciiRange(10, 20),
      w_city:     asciiRange(10, 20),
      w_state:    asciiFixed(2, Alphabet.enUpper),
      w_zip:      asciiFixed(9, Alphabet.num),
      w_tax:      Draw.decimal({ min: Expr.lit(0), max: Expr.lit(0.2), scale: 4 }),
      w_ytd:      Expr.litFloat(300000.0),
    },
  });
}

// District spec: row-index layout r ∈ [0, 10W):
//   d_w_id = r / 10 + 1 ∈ [1, W]
//   d_id   = r % 10 + 1 ∈ [1, 10]
function districtSpec() {
  const dWId = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(DISTRICTS_PER_WAREHOUSE)), Expr.lit(1));
  const dId  = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(DISTRICTS_PER_WAREHOUSE)), Expr.lit(1));
  return Rel.table("district", {
    size: TOTAL_DISTRICTS,
    seed: SEED_DISTRICT,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      d_id:        dId,
      d_w_id:      dWId,
      d_name:      asciiRange(6, 10),
      d_street_1:  asciiRange(10, 20),
      d_street_2:  asciiRange(10, 20),
      d_city:      asciiRange(10, 20),
      d_state:     asciiFixed(2, Alphabet.enUpper),
      d_zip:       asciiFixed(9, Alphabet.num),
      d_tax:       Draw.decimal({ min: Expr.lit(0), max: Expr.lit(0.2), scale: 4 }),
      d_ytd:       Expr.litFloat(30000.0),
      d_next_o_id: Expr.lit(3001),
    },
  });
}

// Customer spec: row-index layout r ∈ [0, 30_000 W):
//   c_w_id = r / 30_000 + 1 ∈ [1, W]
//   c_d_id = (r / 3000) % 10 + 1 ∈ [1, 10]
//   c_id   = r % 3000 + 1 ∈ [1, 3000]
// Spec §4.3.2.3: first 1000 c_ids per district use sequential C_LAST indices
// [0..999] so every name in the 1000-entry dict is guaranteed present in each
// district; remaining 2000 draw via NURand(A=255, x=0, y=999). Without the
// sequential prefix, by-name lookups at tx time (Payment / Order-Status) can
// roll a c_last that no customer in (c_w_id, c_d_id) carries.
// c_credit splits 1:9 BC/GC through Expr.choose.
function customerSpec() {
  const perWh = CUSTOMERS_PER_DISTRICT * DISTRICTS_PER_WAREHOUSE; // 30_000
  const cWId  = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perWh)), Expr.lit(1));
  const cDId  = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(CUSTOMERS_PER_DISTRICT)), Expr.lit(DISTRICTS_PER_WAREHOUSE)),
    Expr.lit(1),
  );
  const cId   = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(CUSTOMERS_PER_DISTRICT)), Expr.lit(1));
  const lastNameDict = Dict.values(C_LAST_DICT);
  const cLastIdx = Expr.if(
    Expr.le(cId, Expr.lit(C_LAST_DICT.length)),
    Expr.sub(cId, Expr.lit(1)),
    Draw.nurand({ a: 255, x: 0, y: 999, cSalt: 0xC1A57 }),
  );
  return Rel.table("customer", {
    size: WAREHOUSES * perWh,
    seed: SEED_CUSTOMER,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      c_id:           cId,
      c_d_id:         cDId,
      c_w_id:         cWId,
      c_first:        asciiRange(8, 16),
      c_middle:       Expr.lit("OE"),
      c_last:         Attr.dictAt(lastNameDict, cLastIdx),
      c_street_1:     asciiRange(10, 20),
      c_street_2:     asciiRange(10, 20),
      c_city:         asciiRange(10, 20),
      c_state:        asciiFixed(2, Alphabet.enUpper),
      c_zip:          asciiFixed(9, Alphabet.num),
      c_phone:        asciiFixed(16, Alphabet.num),
      c_since:        LOAD_TIMESTAMP_EXPR,
      c_credit:       Expr.choose([
        { weight: 1, expr: Expr.lit("BC") },
        { weight: 9, expr: Expr.lit("GC") },
      ]),
      c_credit_lim:   Expr.litFloat(50000.0),
      c_discount:     Draw.decimal({ min: Expr.lit(0), max: Expr.lit(0.5), scale: 4 }),
      c_balance:      Expr.litFloat(-10.0),
      c_ytd_payment:  Expr.litFloat(10.0),
      c_payment_cnt:  Expr.lit(1),
      c_delivery_cnt: Expr.lit(0),
      c_data:         asciiRange(300, 500),
    },
  });
}

// Item spec: i_id = rowIndex()+1 ∈ [1, 100_000].
// Spec §4.3.3.1: i_data is a 26..50 a-string; 10% of rows carry the literal
// "ORIGINAL" at a random position. tpccOriginalOr composes both branches.
function itemSpec() {
  return Rel.table("item", {
    size: ITEMS_PER_WH,
    seed: SEED_ITEM,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      i_id:    Attr.rowId(),
      i_im_id: Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(10_000) }),
      i_name:  asciiRange(14, 24),
      i_price: Draw.decimal({ min: Expr.lit(1.0), max: Expr.lit(100.0), scale: 2 }),
      i_data:  tpccOriginalOr(26, 50),
    },
  });
}

// Stock spec: row-index layout r ∈ [0, 100_000 W):
//   s_w_id = r / 100_000 + 1 ∈ [1, W]
//   s_i_id = r % 100_000 + 1 ∈ [1, 100_000]
function stockSpec() {
  const sWId = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(ITEMS_PER_WH)), Expr.lit(1));
  const sIId = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(ITEMS_PER_WH)), Expr.lit(1));
  // attrs typed as Record<string, PbExpr> via Expr.lit's return type so
  // the s_dist_01..s_dist_10 loop below can append without ceremony.
  type AttrExpr = ReturnType<typeof Expr.lit>;
  const attrs: Record<string, AttrExpr> = {
    s_i_id:     sIId,
    s_w_id:     sWId,
    s_quantity: Draw.intUniform({ min: Expr.lit(10), max: Expr.lit(100) }),
  };
  for (let i = 1; i <= 10; i++) {
    const key = "s_dist_" + String(i).padStart(2, "0");
    attrs[key] = asciiFixed(24);
  }
  attrs.s_ytd        = Expr.lit(0);
  attrs.s_order_cnt  = Expr.lit(0);
  attrs.s_remote_cnt = Expr.lit(0);
  // Spec §4.3.3.1: s_data is a 26..50 a-string; 10% of rows carry the
  // literal "ORIGINAL" at a random position.
  attrs.s_data       = tpccOriginalOr(26, 50);
  return Rel.table("stock", {
    size: TOTAL_STOCK,
    seed: SEED_STOCK,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs,
  });
}

// Orders spec: row-index layout r ∈ [0, 30_000 W):
//   o_w_id = r / 30_000 + 1 ∈ [1, W]
//   o_d_id = (r / 3000) % 10 + 1 ∈ [1, 10]
//   o_id   = r % 3000 + 1 ∈ [1, 3000]
//
// Spec §4.3.3.1:
//   - o_c_id: per-district permutation of [1, 3000]. Realized via
//     std.permuteIndex keyed off (w_id, d_id) so each district's C-ID
//     assignment is a distinct Feistel-shuffled bijection.
//   - o_entry_d: OS-captured load-time timestamp (LOAD_TIMESTAMP_EXPR).
//   - o_carrier_id: NULL for the last 900 rows per district (o_id >
//     2100), else Uniform(1, 10). Expressed as Expr.if + Expr.litNull so
//     the split is deterministic and matches the new_order population by
//     construction.
// Distinct salt per scope so permutation streams for o_c_id are
// uncorrelated with any other per-district key in the workload.
const ORDERS_PERMUTE_SALT = BigInt("0x1BEEF02CACE1DAD1");
function ordersSpec() {
  const perWh = CUSTOMERS_PER_DISTRICT * DISTRICTS_PER_WAREHOUSE; // 30_000
  const oWId  = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perWh)), Expr.lit(1));
  const oDId  = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(CUSTOMERS_PER_DISTRICT)), Expr.lit(DISTRICTS_PER_WAREHOUSE)),
    Expr.lit(1),
  );
  const oId   = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(CUSTOMERS_PER_DISTRICT)), Expr.lit(1));

  // Per-(w_id, d_id) seed: `w_id * 100 + d_id` plus a 64-bit salt so
  // districts across different warehouses don't collide and the seed is
  // uncorrelated with other populations keyed by (w, d). permuteIndex
  // treats the seed as an opaque int64 round-function key — any nonzero
  // value that varies with (w, d) yields a distinct permutation.
  const districtKey = Expr.add(
    Expr.mul(Expr.col("o_w_id"), Expr.lit(100)),
    Expr.col("o_d_id"),
  );
  const permuteSeed = Expr.add(districtKey, Expr.lit(ORDERS_PERMUTE_SALT));
  const oCId = Expr.add(
    std.permuteIndex(
      permuteSeed,
      Expr.sub(Expr.col("o_id"), Expr.lit(1)),
      Expr.lit(CUSTOMERS_PER_DISTRICT),
    ),
    Expr.lit(1),
  );

  // o_carrier_id: NULL for the undelivered tail, otherwise Uniform(1,10).
  const oCarrierId = Expr.if(
    Expr.gt(Expr.col("o_id"), Expr.lit(ORDERS_DELIVERED)),
    Expr.litNull(),
    Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(10) }),
  );

  return Rel.table("orders", {
    size: WAREHOUSES * perWh,
    seed: SEED_ORDERS,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      o_id:         oId,
      o_d_id:       oDId,
      o_w_id:       oWId,
      o_c_id:       oCId,
      o_entry_d:    LOAD_TIMESTAMP_EXPR,
      o_carrier_id: oCarrierId,
      o_ol_cnt:     Expr.lit(OL_CNT_FIXED),
      o_all_local:  Expr.lit(1),
    },
  });
}

// Order_line spec: row-index layout r ∈ [0, 300_000 W), 10 lines per
// (o_w_id, o_d_id, o_id) in orders:
//   ol_w_id   = r / 300_000 + 1 ∈ [1, W]
//   ol_d_id   = (r / 30_000) % 10 + 1 ∈ [1, 10]
//   ol_o_id   = (r / 10) % 3000 + 1 ∈ [1, 3000]
//   ol_number = r % 10 + 1 ∈ [1, 10]
// FK integrity against orders is exact because every parent (o_w_id,
// o_d_id, o_id) has exactly 10 children at matching indices.
function orderLineSpec() {
  const perDWh = CUSTOMERS_PER_DISTRICT * DISTRICTS_PER_WAREHOUSE * OL_CNT_FIXED; // 300_000
  const perD   = CUSTOMERS_PER_DISTRICT * OL_CNT_FIXED;                           // 30_000
  const olWId  = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perDWh)), Expr.lit(1));
  const olDId  = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(perD)), Expr.lit(DISTRICTS_PER_WAREHOUSE)),
    Expr.lit(1),
  );
  const olOId  = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(OL_CNT_FIXED)), Expr.lit(CUSTOMERS_PER_DISTRICT)),
    Expr.lit(1),
  );
  const olNum  = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(OL_CNT_FIXED)), Expr.lit(1));

  // Spec §4.3.3.1:
  //   - ol_delivery_d: NULL for undelivered orders (ol_o_id > 2100), else
  //     the OS-captured load timestamp.
  //   - ol_amount: Uniform(0.01, 9999.99) for undelivered rows, 0.00 for
  //     delivered rows.
  const undelivered = Expr.gt(Expr.col("ol_o_id"), Expr.lit(ORDERS_DELIVERED));
  const olDeliveryD = Expr.if(undelivered, Expr.litNull(), LOAD_TIMESTAMP_EXPR);
  const olAmount    = Expr.if(
    undelivered,
    Draw.decimal({ min: Expr.lit(0.01), max: Expr.lit(9999.99), scale: 2 }),
    Expr.litFloat(0.0),
  );

  return Rel.table("order_line", {
    size: WAREHOUSES * perDWh,
    seed: SEED_ORDER_LINE,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      ol_o_id:        olOId,
      ol_d_id:        olDId,
      ol_w_id:        olWId,
      ol_number:      olNum,
      ol_i_id:        Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(ITEMS_PER_WH) }),
      ol_supply_w_id: olWId,
      ol_delivery_d:  olDeliveryD,
      ol_quantity:    Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(5) }),
      ol_amount:      olAmount,
      ol_dist_info:   asciiFixed(24),
    },
  });
}

// New_order spec: last 900 o_ids per district per warehouse.
// Row-index layout r ∈ [0, 9000 W):
//   no_w_id = r / 9000 + 1 ∈ [1, W]
//   no_d_id = (r / 900) % 10 + 1 ∈ [1, 10]
//   no_o_id = r % 900 + 2101 ∈ [2101, 3000]
function newOrderSpec() {
  const perWh = ORDERS_UNDELIVERED * DISTRICTS_PER_WAREHOUSE; // 9000
  const noWId = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(perWh)), Expr.lit(1));
  const noDId = Expr.add(
    Expr.mod(Expr.div(Attr.rowIndex(), Expr.lit(ORDERS_UNDELIVERED)), Expr.lit(DISTRICTS_PER_WAREHOUSE)),
    Expr.lit(1),
  );
  const noOId = Expr.add(Expr.mod(Attr.rowIndex(), Expr.lit(ORDERS_UNDELIVERED)), Expr.lit(ORDERS_DELIVERED + 1));
  return Rel.table("new_order", {
    size: WAREHOUSES * perWh,
    seed: SEED_NEW_ORDER,
    method: DatagenInsertMethod.NATIVE,
    parallelism: LOAD_WORKERS || undefined,
    attrs: {
      no_o_id: noOId,
      no_d_id: noDId,
      no_w_id: noWId,
    },
  });
}

export function setup() {
  Step("drop_schema", () => {
    sql("drop_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_schema", () => {
    sql("create_schema").forEach((query) => driver.exec(query, {}));
  });

  // Single bulk-load step covering all nine TPC-C tables. Each call feeds
  // an InsertSpec into the new datagen runtime via driver.insertSpec;
  // FK-friendly order (warehouse → district → customer → item → stock →
  // orders → order_line → new_order) matches the PG REFERENCES constraints.
  Step("load_data", () => {
    driver.insertSpec(warehouseSpec());
    driver.insertSpec(districtSpec());
    driver.insertSpec(customerSpec());
    driver.insertSpec(itemSpec());
    driver.insertSpec(stockSpec());
    driver.insertSpec(ordersSpec());
    driver.insertSpec(orderLineSpec());
    driver.insertSpec(newOrderSpec());
    // history is empty at load time (spec §4.3.4 initial cardinality 0).
  });

  // Spec §3.3.2 CC1-CC4 + §4.3.4 cardinalities + §4.3.3.1 distribution rules.
  // Fails setup() hard if any assertion trips so downstream transaction
  // runs cannot execute on silently-broken data.
  //
  // Portability: CC2/CC3 originally use scalar-subquery subtraction and
  // correlated MAX, which YDB's YQL rejects (it expects Module::Func
  // syntax inside subquery contexts). We fetch aggregates with plain
  // `SELECT ... GROUP BY` and fold the per-district comparisons in JS;
  // every dialect supports the flat shape. `LIKE '%ORIGINAL%'` scans
  // over item/stock can be expensive on sbroad's default vdbe opcode
  // budget — the stroppy-playground compose bumps the limit cluster-wide
  // (see README); locally `make tmpfs-up` is fine for WAREHOUSES=1.
  Step("validate_population", () => {
    const TOTAL_ORDERS     = TOTAL_DISTRICTS * CUSTOMERS_PER_DISTRICT;
    const TOTAL_NEW_ORDER  = TOTAL_DISTRICTS * ORDERS_UNDELIVERED;
    const TOTAL_ORDER_LINE = TOTAL_ORDERS * OL_CNT_FIXED;
    const TOTAL_CUSTOMERS  = TOTAL_ORDERS; // 3000 per district

    type DistRow = { dNextOId: number };
    type NoStats = { maxNoOId: number; minNoOId: number; cnt: number };

    const dKey = (w: unknown, d: unknown) => `${Number(w)}/${Number(d)}`;
    const distMap: Record<string, DistRow> = {};
    const ordMaxMap: Record<string, number> = {};
    const noStatsMap: Record<string, NoStats> = {};

    let cc1WSum = NaN, cc1DSum = NaN;
    let cc4OSum = NaN, cc4OlCnt = NaN;

    try {
      for (const r of driver.queryRows("SELECT d_w_id, d_id, d_next_o_id FROM district")) {
        distMap[dKey(r[0], r[1])] = { dNextOId: Number(r[2]) };
      }
      for (const r of driver.queryRows(
        "SELECT o_w_id, o_d_id, MAX(o_id) FROM orders GROUP BY o_w_id, o_d_id",
      )) {
        ordMaxMap[dKey(r[0], r[1])] = Number(r[2]);
      }
      for (const r of driver.queryRows(
        "SELECT no_w_id, no_d_id, MAX(no_o_id), MIN(no_o_id), COUNT(*) FROM new_order GROUP BY no_w_id, no_d_id",
      )) {
        noStatsMap[dKey(r[0], r[1])] = {
          maxNoOId: Number(r[2]),
          minNoOId: Number(r[3]),
          cnt:      Number(r[4]),
        };
      }
      cc1WSum  = Number(driver.queryValue("SELECT SUM(w_ytd) FROM warehouse"));
      cc1DSum  = Number(driver.queryValue("SELECT SUM(d_ytd) FROM district"));
      cc4OSum  = Number(driver.queryValue("SELECT SUM(o_ol_cnt) FROM orders"));
      cc4OlCnt = Number(driver.queryValue("SELECT COUNT(*) FROM order_line"));
    } catch (e) {
      throw new Error(`validate_population: prefetch failed: ${e}`);
    }

    const evalCc2a = (): { ok: boolean; detail: string } => {
      for (const k in distMap) {
        const want = distMap[k].dNextOId - 1;
        const got = ordMaxMap[k];
        if (got !== want) return { ok: false, detail: `district ${k}: d_next_o_id-1=${want}, max(o_id)=${got}` };
      }
      return { ok: true, detail: "" };
    };
    const evalCc2b = (): { ok: boolean; detail: string } => {
      for (const k in distMap) {
        const om = ordMaxMap[k];
        const ns = noStatsMap[k];
        const noMax = ns ? ns.maxNoOId : undefined;
        if (om !== noMax) return { ok: false, detail: `district ${k}: max(o_id)=${om}, max(no_o_id)=${noMax}` };
      }
      return { ok: true, detail: "" };
    };
    const evalCc3 = (): { ok: boolean; detail: string } => {
      for (const k in distMap) {
        const ns = noStatsMap[k];
        if (!ns) return { ok: false, detail: `district ${k}: missing new_order stats` };
        if (ns.maxNoOId - ns.minNoOId + 1 !== ns.cnt) {
          return { ok: false, detail: `district ${k}: max-min+1=${ns.maxNoOId - ns.minNoOId + 1} vs count=${ns.cnt}` };
        }
      }
      return { ok: true, detail: "" };
    };

    type QueryCheck    = { name: string; query: string; ok: (v: unknown) => boolean };
    type ComputedCheck = { name: string; computed: () => { ok: boolean; detail: string } };
    type Check         = QueryCheck | ComputedCheck;

    const checks: Check[] = [
      // --- §4.3.4 initial cardinalities ---
      { name: `ITEM = ${ITEMS}`,
        query: "SELECT COUNT(*) FROM item",
        ok: v => Number(v) === ITEMS },
      { name: `WAREHOUSE = ${WAREHOUSES}`,
        query: "SELECT COUNT(*) FROM warehouse",
        ok: v => Number(v) === WAREHOUSES },
      { name: `DISTRICT = ${TOTAL_DISTRICTS}`,
        query: "SELECT COUNT(*) FROM district",
        ok: v => Number(v) === TOTAL_DISTRICTS },
      { name: `CUSTOMER = ${TOTAL_CUSTOMERS}`,
        query: "SELECT COUNT(*) FROM customer",
        ok: v => Number(v) === TOTAL_CUSTOMERS },
      { name: `STOCK = ${TOTAL_STOCK}`,
        query: "SELECT COUNT(*) FROM stock",
        ok: v => Number(v) === TOTAL_STOCK },
      { name: `ORDERS = ${TOTAL_ORDERS}`,
        query: "SELECT COUNT(*) FROM orders",
        ok: v => Number(v) === TOTAL_ORDERS },
      { name: `NEW_ORDER = ${TOTAL_NEW_ORDER}`,
        query: "SELECT COUNT(*) FROM new_order",
        ok: v => Number(v) === TOTAL_NEW_ORDER },
      { name: `ORDER_LINE = ${TOTAL_ORDER_LINE}`,
        query: "SELECT COUNT(*) FROM order_line",
        ok: v => Number(v) === TOTAL_ORDER_LINE },

      // --- §3.3.2 CC1: sum(W_YTD) == sum(D_YTD) ---
      { name: "CC1 sum(W_YTD) = sum(D_YTD)",
        computed: () => Math.abs(cc1WSum - cc1DSum) < 0.01
          ? { ok: true, detail: "" }
          : { ok: false, detail: `sum(w_ytd)=${cc1WSum}, sum(d_ytd)=${cc1DSum}` } },

      // --- §3.3.2 CC2: D_NEXT_O_ID - 1 = max(O_ID) = max(NO_O_ID) per district ---
      { name: "CC2a D_NEXT_O_ID - 1 = max(O_ID) per district",
        computed: evalCc2a },
      { name: "CC2b max(O_ID) = max(NO_O_ID) per district",
        computed: evalCc2b },

      // --- §3.3.2 CC3: max(NO_O_ID) - min(NO_O_ID) + 1 = count(new_order) per district ---
      { name: "CC3 new_order contiguous range per district",
        computed: evalCc3 },

      // --- §3.3.2 CC4: sum(O_OL_CNT) = count(ORDER_LINE) ---
      { name: "CC4 sum(O_OL_CNT) = count(order_line)",
        computed: () => cc4OSum === cc4OlCnt
          ? { ok: true, detail: "" }
          : { ok: false, detail: `sum(o_ol_cnt)=${cc4OSum}, count(order_line)=${cc4OlCnt}` } },

      // --- §4.3.3.1 distribution rules (5% tolerance — spec allows modest skew) ---
      { name: "I_DATA 10% contains ORIGINAL (5..15%)",
        query: "SELECT 100.0 * SUM(CASE WHEN i_data LIKE '%ORIGINAL%' THEN 1 ELSE 0 END) / COUNT(*) FROM item",
        ok: v => Number(v) >= 5 && Number(v) <= 15 },
      { name: "S_DATA 10% contains ORIGINAL (5..15%)",
        query: "SELECT 100.0 * SUM(CASE WHEN s_data LIKE '%ORIGINAL%' THEN 1 ELSE 0 END) / COUNT(*) FROM stock",
        ok: v => Number(v) >= 5 && Number(v) <= 15 },
      { name: "C_CREDIT 10% BC (5..15%)",
        query: "SELECT 100.0 * SUM(CASE WHEN c_credit = 'BC' THEN 1 ELSE 0 END) / COUNT(*) FROM customer",
        ok: v => Number(v) >= 5 && Number(v) <= 15 },

      // --- fixed-value sanity checks ---
      { name: "C_MIDDLE = 'OE' everywhere",
        query: "SELECT COUNT(*) FROM customer WHERE c_middle <> 'OE'",
        ok: v => Number(v) === 0 },
      { name: "W_YTD = 300000 everywhere",
        query: "SELECT COUNT(*) FROM warehouse WHERE w_ytd <> 300000",
        ok: v => Number(v) === 0 },
      { name: "D_NEXT_O_ID = 3001 everywhere",
        query: "SELECT COUNT(*) FROM district WHERE d_next_o_id <> 3001",
        ok: v => Number(v) === 0 },
    ];

    const failures: string[] = [];
    for (const c of checks) {
      if ("query" in c) {
        let v: unknown;
        try {
          v = driver.queryValue(c.query);
        } catch (e) {
          const msg = `  ✗ ${c.name}: query error: ${e}`;
          console.error(msg);
          failures.push(msg);
          continue;
        }
        if (c.ok(v)) {
          console.log(`  ✓ ${c.name}`);
        } else {
          const msg = `  ✗ ${c.name}: got ${v}`;
          console.error(msg);
          failures.push(msg);
        }
      } else {
        let res: { ok: boolean; detail: string };
        try {
          res = c.computed();
        } catch (e) {
          const msg = `  ✗ ${c.name}: compute error: ${e}`;
          console.error(msg);
          failures.push(msg);
          continue;
        }
        if (res.ok) {
          console.log(`  ✓ ${c.name}`);
        } else {
          const msg = `  ✗ ${c.name}: ${res.detail}`;
          console.error(msg);
          failures.push(msg);
        }
      }
    }
    if (failures.length > 0) {
      throw new Error(
        `validate_population: ${failures.length} check(s) failed:\n${failures.join("\n")}`,
      );
    }
  });

  Step.begin("workload");
}

// =====================================================================
// NEW_ORDER (45% of mix)
// Spec §2.4:
//   - §2.4.1.1: w_id is the terminal's fixed home warehouse (HOME_W_ID).
//   - §2.4.1.4: 99% of lines are local (supply_w_id = w_id); 1% are remote.
//   - §2.4.1.5: c_id ~ NURand(1023, 1, 3000); ol_i_id ~ NURand(8191, 1, 100000).
//   - §2.4.2.3: 1% of transactions end in rollback via a bogus last-line i_id.
//   - §2.4.2.2: read customer/warehouse/district → increment d_next_o_id →
//               for each line: get item, get stock, update stock, insert OL.
// =====================================================================
const newordDIdGen      = DrawRT.intUniform(seedOf("neword.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const newordCIdGen      = DrawRT.nurand(seedOf("neword.c_id"), 1023, 1, CUSTOMERS_PER_DISTRICT);
const newordOOlCntGen   = DrawRT.intUniform(seedOf("neword.ol_cnt"), 5, 15);
const newordItemIdGen   = DrawRT.nurand(seedOf("neword.item_id"), 8191, 1, ITEMS);
const newordQuantityGen = DrawRT.intUniform(seedOf("neword.quantity"), 1, 10);
// Use int32(1, 100) + threshold compare rather than bool(0.01) so that the
// seeded stream is deterministic and matches what the report compliance
// checker expects (1% rollback, 1% remote).
const newordRemoteLineGen = DrawRT.intUniform(seedOf("neword.remote_line"), 1, 100);  // <=1 ⇒ remote supply warehouse
const newordRollbackGen   = DrawRT.intUniform(seedOf("neword.rollback"), 1, 100);     // <=1 ⇒ force rollback via bogus i_id

function new_order() {
  tpccNewOrderTotal.add(1);
  const t0 = Date.now();

  const w_id   = HOME_W_ID;
  const d_id   = newordDIdGen.next() as number;
  const c_id   = newordCIdGen.next() as number;
  const ol_cnt = newordOOlCntGen.next() as number;

  // Pre-materialise per-line item ids, quantities, and supply warehouses so
  // all_local is known before we insert the order header. Spec §2.4.1.5:
  // each ol_i_id is drawn independently via NURand; §2.4.1.4: each line has
  // an independent 1% chance of being remote.
  const line_i_id:     number[] = new Array(ol_cnt);
  const line_qty:      number[] = new Array(ol_cnt);
  const line_supply:   number[] = new Array(ol_cnt);
  let   all_local = 1;
  let   remote_line_cnt = 0;

  for (let i = 0; i < ol_cnt; i++) {
    line_i_id[i] = newordItemIdGen.next() as number;
    line_qty[i]  = newordQuantityGen.next() as number;
    tpccRemoteLineTotal.add(1);
    const is_remote = WAREHOUSES > 1 && (newordRemoteLineGen.next() as number) <= 1;
    if (is_remote) {
      tpccRemoteLineRem.add(1);
      line_supply[i] = pickRemoteWh();
      all_local = 0;
      remote_line_cnt++;
    } else {
      line_supply[i] = w_id;
    }
  }

  // Spec §2.4.2.3: 1% of new_order transactions should be rolled back by
  // submitting an unknown item id on the LAST line. We signal via a sentinel
  // value (ITEMS + 1) and detect it inside the loop to throw a rollback
  // sentinel error. We use driver.begin()/commit()/rollback() directly so the
  // forced-rollback path doesn't inflate tx_error_rate — the user-driven
  // rollback is spec-mandated "success" behaviour.
  //
  // Isolation "none" (picodata) short-circuits Begin/Commit/Rollback to
  // no-ops, so throwing mid-loop would leave partial inserts behind. Drain
  // the decision generator to keep its stream aligned across drivers, but
  // skip the actual sentinel in NONE mode — the tx would have committed
  // anyway and the compliance numbers are reported via tpcc_rollback_*.
  const rollback_roll = (newordRollbackGen.next() as number) <= 1;
  const force_rollback = rollback_roll && TX_ISOLATION !== "none";
  if (force_rollback) {
    tpccRollbackDecided.add(1);
    line_i_id[ol_cnt - 1] = ITEMS + 1;
  }

  // T2.3: wrap the tx body in tpccRetry so PG snapshot-isolation aborts
  // (SQLSTATE 40001) replay against a fresh snapshot. Pre-tx random picks
  // (line ids, force_rollback decision, counters) stay OUTSIDE the loop —
  // a retry replays the SAME logical transaction, not a different one. The
  // spec §2.4.2.3 rollback sentinel is filtered out by isSerializationError
  // (its `tpcc_rollback:` prefix short-circuits the regex), so the rollback
  // path always escapes the retry loop on the first attempt as before.
  try {
    tpccRetry(() => {
      const tx = driver.begin({ isolation: TX_ISOLATION });
      try {
        // Read customer discount / credit and warehouse tax (real round-trip).
        tx.queryRow(sql("workload_tx_new_order", "get_customer")!, { c_id, d_id, w_id });
        tx.queryRow(sql("workload_tx_new_order", "get_warehouse")!, { w_id });

        // Read district next_o_id + tax → use as o_id, then bump d_next_o_id.
        const distRow = tx.queryRow(sql("workload_tx_new_order", "get_district")!, { d_id, w_id });
        if (!distRow) {
          throw new Error(`new_order: district (${w_id},${d_id}) not found`);
        }
        const o_id = Number(distRow[0]);
        tx.exec(sql("workload_tx_new_order", "update_district")!, { d_id, w_id });

        tx.exec(sql("workload_tx_new_order", "insert_order")!, {
          o_id, d_id, w_id, c_id, ol_cnt, all_local,
        });
        tx.exec(sql("workload_tx_new_order", "insert_new_order")!, { o_id, d_id, w_id });

        // Layer 2: batch-read items and stocks to cut ~18 round-trips per
        // new_order (10 item reads + 10 stock reads → 1 + ~1 batch reads).
        // Per-line stock UPDATEs and OL INSERTs remain individual queries.

        // --- batch item read ---
        // Deduplicate item IDs; NURand can produce duplicates across lines.
        const uniqueItemIds = [...new Set(line_i_id)];
        const itemTemplate = sql("workload_tx_new_order", "get_items_batch")!;
        const itemQuery = itemTemplate.sql.replace(/\{item_ids\}/g, uniqueItemIds.join(","));
        const itemRows = tx.queryRows(itemQuery, {});
        // Index by i_id. Batch result columns: [0]=i_id, [1]=i_price, [2]=i_name, [3]=i_data.
        const itemMap = new Map<number, any[]>();
        for (const row of itemRows) {
          itemMap.set(Number(row[0]), row);
        }

        // Spec §2.4.2.3 rollback check: if force_rollback is set, the
        // last line's i_id (ITEMS+1) won't appear in itemMap.
        if (force_rollback && !itemMap.has(line_i_id[ol_cnt - 1])) {
          tpccRollbackDone.add(1);
          throw new Error("tpcc_rollback:item_not_found");
        }

        // --- batch stock read ---
        // Group item IDs by supply warehouse. Typically one group (home
        // warehouse); ~1% of lines pick a remote warehouse.
        const stockByWh = new Map<number, Set<number>>();
        for (let i = 0; i < ol_cnt; i++) {
          if (!itemMap.has(line_i_id[i])) continue;
          const sw = line_supply[i];
          let s = stockByWh.get(sw);
          if (!s) { s = new Set(); stockByWh.set(sw, s); }
          s.add(line_i_id[i]);
        }
        // Key: "supply_w_id/i_id". Batch result columns:
        // [0]=s_i_id, [1]=s_quantity, [2]=s_data, [3..12]=s_dist_01..s_dist_10.
        const stockMap = new Map<string, any[]>();
        const stockTemplate = sql("workload_tx_new_order", "get_stocks_batch")!;
        for (const [sw, iids] of stockByWh) {
          const q = stockTemplate.sql.replace(/\{item_ids\}/g, [...iids].join(","));
          const rows = tx.queryRows(q, { w_id: sw });
          for (const row of rows) {
            stockMap.set(`${sw}/${Number(row[0])}`, row);
          }
        }

        // --- per-line: update stock + insert order_line ---
        for (let ol_number = 1; ol_number <= ol_cnt; ol_number++) {
          const i_id        = line_i_id[ol_number - 1];
          const ol_quantity = line_qty[ol_number - 1];
          const supply_w_id = line_supply[ol_number - 1];

          const itemRow = itemMap.get(i_id);
          if (!itemRow) {
            // Should only happen for the rollback sentinel (caught above).
            tpccRollbackDone.add(1);
            throw new Error("tpcc_rollback:item_not_found");
          }
          // Batch columns: [0]=i_id, [1]=i_price, [2]=i_name, [3]=i_data.
          const i_price = Number(itemRow[1]);

          const stockKey = `${supply_w_id}/${i_id}`;
          const stockRow = stockMap.get(stockKey);
          if (!stockRow) continue;
          // Batch columns: [0]=s_i_id, [1]=s_quantity, ...
          // s_dist_NN: d_id=1 → col 3 (s_dist_01), d_id=10 → col 12.
          const s_quantity_old = Number(stockRow[1]);
          const dist_info      = String(stockRow[d_id + 2] ?? "");
          const new_quantity   =
            s_quantity_old - ol_quantity >= 10
              ? s_quantity_old - ol_quantity
              : s_quantity_old - ol_quantity + 91;

          // Update cached s_quantity so duplicate (supply_w_id, i_id) pairs
          // across order lines see the post-update value, not the stale one.
          stockRow[1] = new_quantity;

          const remote_cnt = supply_w_id !== w_id ? 1 : 0;

          tx.exec(sql("workload_tx_new_order", "update_stock")!, {
            quantity: new_quantity, ol_quantity, remote_cnt, i_id, w_id: supply_w_id,
          });

          const amount = Math.round(ol_quantity * i_price * 100) / 100;

          tx.exec(sql("workload_tx_new_order", "insert_order_line")!, {
            o_id, d_id, w_id, ol_number, i_id, supply_w_id,
            quantity: ol_quantity, amount, dist_info,
          });
        }

        tx.commit();
      } catch (e) {
        // Always roll back the failed attempt. If this is a retryable
        // serialization error, tpccRetry catches it next and starts a fresh
        // tx; if it's the §2.4.2.3 rollback sentinel, we swallow it after
        // the rollback so the retry helper sees a successful return.
        try { tx.rollback(); } catch (_) { /* ignore */ }
        const msg = (e as Error)?.message ?? String(e);
        if (msg.startsWith("tpcc_rollback:")) {
          return; // spec-mandated rollback — counts as success, no retry
        }
        throw e;
      }
    });
  } finally {
    // T3.2: record total user-visible elapsed (including retries) into the
    // p90 trend. Recorded in finally so error tails still feed the metric
    // and can flag slow-query regressions, matching the prior behaviour.
    tpccNewOrderDuration.add(Date.now() - t0);
  }
  // Reference remote_line_cnt to satisfy noUnusedLocals; useful for future
  // post-run audit of §2.4.1.4 compliance.
  void remote_line_cnt;
}

// =====================================================================
// PAYMENT (43% of mix)
// Spec §2.5:
//   - §2.5.1.1: w_id is the terminal's fixed home warehouse (HOME_W_ID).
//   - §2.5.1.2: 85% home customer, 15% remote. For remote, c_w_id is
//               picked uniformly from the OTHER warehouses; c_d_id is
//               picked uniformly in [1, 10] (independent of d_id).
//   - §2.5.1.2: 60% of lookups use (c_w_id, c_d_id, c_last), 40% by c_id.
//               c_last picked via NURand(255, 0, 999) into C_LAST_DICT;
//               c_id drawn via NURand(1023, 1, 3000).
// =====================================================================
const paymentDIdGen     = DrawRT.intUniform(seedOf("payment.d_id"),  1, DISTRICTS_PER_WAREHOUSE);
const paymentCDIdGen    = DrawRT.intUniform(seedOf("payment.c_d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const paymentCIdGen     = DrawRT.nurand(seedOf("payment.c_id"), 1023, 1, CUSTOMERS_PER_DISTRICT);
const paymentHAmountGen = DrawRT.floatUniform(seedOf("payment.h_amount"), 1, 5000);
const paymentHDataGen   = DrawRT.ascii(seedOf("payment.h_data"), 12, 24, Alphabet.enSpc);
// 15% remote. <=15 on a uniform [1,100] gives 15% exactly.
const paymentRemoteGen  = DrawRT.intUniform(seedOf("payment.remote"), 1, 100);
// 60% by-name. <=60 on a uniform [1,100].
const paymentBynameGen  = DrawRT.intUniform(seedOf("payment.byname"), 1, 100);

function payment() {
  tpccPaymentTotal.add(1);
  const t0 = Date.now();

  const w_id   = HOME_W_ID;
  const d_id   = paymentDIdGen.next() as number;
  const amount = paymentHAmountGen.next();
  const h_data = paymentHDataGen.next();
  const h_id   = nextHid();

  // Spec §2.5.1.2: 85% home (c_w_id = w_id, c_d_id = d_id), 15% remote
  // (c_w_id ≠ w_id, c_d_id uniform over [1,10]). With a single warehouse
  // the remote clause degenerates to home.
  const is_remote = WAREHOUSES > 1 && (paymentRemoteGen.next() as number) <= 15;
  if (is_remote) tpccPaymentRemote.add(1);
  const c_w_id = is_remote ? pickRemoteWh() : w_id;
  const c_d_id = is_remote ? (paymentCDIdGen.next() as number) : d_id;

  // Spec §2.5.1.2: 60% by-name, 40% by-id. The by-name c_last is drawn
  // from C_LAST_DICT via NURand(255, 0, 999), matching the load phase
  // (§4.3.2.3) so lookups hit the populated syllable strings.
  const is_byname = (paymentBynameGen.next() as number) <= 60;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";
  // Keep the by-id stream deterministic even when the roll chooses
  // by-name — drain the generator so a mid-run roll switch doesn't
  // shift subsequent c_ids.
  const c_id_pick = paymentCIdGen.next() as number;

  // T2.3: capture BC-credit branch in a closure-scoped flag so the counter
  // can be incremented exactly ONCE outside the retry loop, regardless of
  // how many serialization-failure retries fired. tpccPaymentByname /
  // tpccPaymentRemote are already incremented outside (they depend only
  // on the pre-tx random rolls), so they're safe.
  let payment_was_bc = false;
  try {
    tpccRetry(() => {
      payment_was_bc = false; // reset across attempts so the final attempt's branch wins
      driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
        // Layer 1: pg/ydb merge UPDATE + SELECT into a single
        // UPDATE...RETURNING round-trip; mysql/pico keep the two-query path.
        let w_name: string;
        if (HAS_RETURNING) {
          const whRow = tx.queryRow(sql("workload_tx_payment", "update_get_warehouse")!, { w_id, amount });
          if (!whRow) throw new Error(`payment: warehouse ${w_id} not found`);
          w_name = String(whRow[0] ?? "");
        } else {
          tx.exec(sql("workload_tx_payment", "update_warehouse")!, { w_id, amount });
          const whRow = tx.queryRow(sql("workload_tx_payment", "get_warehouse")!, { w_id });
          if (!whRow) throw new Error(`payment: warehouse ${w_id} not found`);
          w_name = String(whRow[0] ?? "");
        }

        let d_name: string;
        if (HAS_RETURNING) {
          const distRow = tx.queryRow(sql("workload_tx_payment", "update_get_district")!, { w_id, d_id, amount });
          if (!distRow) throw new Error(`payment: district (${w_id},${d_id}) not found`);
          d_name = String(distRow[0] ?? "");
        } else {
          tx.exec(sql("workload_tx_payment", "update_district")!, { w_id, d_id, amount });
          const distRow = tx.queryRow(sql("workload_tx_payment", "get_district")!, { w_id, d_id });
          if (!distRow) throw new Error(`payment: district (${w_id},${d_id}) not found`);
          d_name = String(distRow[0] ?? "");
        }

        // Spec §2.5.2.2: read customer balance / credit. Either by c_id
        // (40%) or by c_last (60%) — the by-name branch takes two queries
        // (count + median SELECT) while the by-id branch is one. Both
        // SELECTs also return c_credit (2-char) and c_data (up to 500
        // chars) for the §1.8 BC-credit append path below.
        let c_id: number;
        let c_credit: string;
        let c_data_old: string;
        if (is_byname) {
          const cntRaw = tx.queryValue(
            sql("workload_tx_payment", "count_customers_by_name")!,
            { w_id: c_w_id, d_id: c_d_id, c_last: c_last_pick },
          );
          const nameCount = Number(cntRaw ?? 0);
          if (nameCount === 0) {
            // No rows for the rolled c_last — treat like any customer-miss
            // (should never happen once §4.3.2.3 load is verified, but stay
            // defensive so a loader regression surfaces as a failed tx, not
            // a silent no-op).
            throw new Error(`payment: no customers match c_last='${c_last_pick}' in (${c_w_id},${c_d_id})`);
          }
          const offset = Math.floor((nameCount - 1) / 2);
          let nameRow: any[] | undefined;
          if (IS_PICODATA) {
            // picodata/sbroad rejects OFFSET — fetch all matching rows and
            // pick the median client-side. Group size is tiny (~3 on avg).
            const allRows = tx.queryRows(
              sql("workload_tx_payment", "get_customer_by_name")!,
              { w_id: c_w_id, d_id: c_d_id, c_last: c_last_pick },
            );
            nameRow = allRows[offset];
          } else {
            nameRow = tx.queryRow(
              sql("workload_tx_payment", "get_customer_by_name")!,
              { w_id: c_w_id, d_id: c_d_id, c_last: c_last_pick, offset },
            );
          }
          if (!nameRow) {
            throw new Error(`payment: by-name SELECT returned no row for c_last='${c_last_pick}'`);
          }
          // Column order (see pg/mysql/pico/ydb sql):
          // [c_id, c_first, c_middle, c_last, c_street_1, c_street_2,
          //  c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim,
          //  c_discount, c_balance, c_since, c_data]
          c_id = Number(nameRow[0]);
          c_credit = String(nameRow[10] ?? "").trim();
          c_data_old = String(nameRow[15] ?? "");
        } else {
          c_id = c_id_pick;
          const custRow = tx.queryRow(sql("workload_tx_payment", "get_customer_by_id")!, {
            w_id: c_w_id, d_id: c_d_id, c_id,
          });
          if (!custRow) throw new Error(`payment: customer ${c_id} not found`);
          // Column order (see pg/mysql/pico/ydb sql):
          // [c_first, c_middle, c_last, c_street_1, c_street_2, c_city,
          //  c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount,
          //  c_balance, c_since, c_data]
          c_credit = String(custRow[9] ?? "").trim();
          c_data_old = String(custRow[14] ?? "");
        }

        // Spec §2.5.2.2: if c_credit = 'BC', the Payment tx must prepend
        // the current transaction's identifying tuple to c_data and
        // truncate to 500 chars; GC customers (90%) keep c_data untouched.
        // We build the prefix AND clamp client-side so the SQL UPDATE just
        // assigns a fixed-length string. This sidesteps per-dialect '||' vs
        // CONCAT() differences AND YDB's Substring(String) vs Utf8 mismatch.
        if (c_credit === "BC") {
          const amountStr = (amount as number).toFixed(2);
          const c_data_new = `${c_id} ${c_d_id} ${c_w_id} ${d_id} ${w_id} ${amountStr}|${c_data_old}`.slice(0, 500);
          tx.exec(sql("workload_tx_payment", "update_customer_bc")!, {
            w_id: c_w_id, d_id: c_d_id, c_id, amount, c_data_new,
          });
          payment_was_bc = true;
        } else {
          tx.exec(sql("workload_tx_payment", "update_customer")!, {
            w_id: c_w_id, d_id: c_d_id, c_id, amount,
          });
        }

        // Spec §2.5.2.2: h_data = w_name + "    " + d_name, truncated to 24.
        const h_data_full = (w_name + "    " + d_name).slice(0, 24) || h_data;
        tx.exec(sql("workload_tx_payment", "insert_history")!, {
          h_id, h_c_id: c_id, h_c_d_id: c_d_id, h_c_w_id: c_w_id,
          h_d_id: d_id, h_w_id: w_id, h_amount: amount, h_data: h_data_full,
        });
      });
    });
    if (is_byname) tpccPaymentByname.add(1);
    if (payment_was_bc) tpccPaymentBc.add(1);
  } finally {
    tpccPaymentDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// ORDER_STATUS (4% of mix) — read-only.
// Spec §2.6: read customer → read their last order → read that order's lines.
// The o_id for get_order_lines must come from the last-order SELECT, not
// from a random counter.
//   - §2.6.1.1: w_id is the terminal's fixed home warehouse.
//   - §2.6.1.2: 60% by-name / 40% by-id. c_id ~ NURand(1023, 1, 3000);
//               c_last via NURand(255, 0, 999) into C_LAST_DICT.
// =====================================================================
const ostatDIdGen    = DrawRT.intUniform(seedOf("ostat.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const ostatCIdGen    = DrawRT.nurand(seedOf("ostat.c_id"), 1023, 1, CUSTOMERS_PER_DISTRICT);
const ostatBynameGen = DrawRT.intUniform(seedOf("ostat.byname"), 1, 100);

function order_status() {
  tpccOrderStatusTotal.add(1);
  const t0 = Date.now();
  const w_id = HOME_W_ID;
  const d_id = ostatDIdGen.next() as number;
  // Drain both generators regardless of the by-name roll to keep the
  // per-VU random stream alignment stable run-over-run.
  const c_id_pick = ostatCIdGen.next() as number;
  const is_byname = (ostatBynameGen.next() as number) <= 60;
  const c_last_pick = is_byname ? C_LAST_DICT[nurand255Gen.next() as number] : "";

  // T2.3: tpccOrderStatusByname depends only on the pre-tx is_byname roll
  // (the `return` paths inside the tx happen because the customer has no
  // matching row, not because the by-name decision changes), so move the
  // counter increment outside the retry — and only fire it if the inner
  // body actually completed the by-name SELECT successfully. Capture that
  // via a closure flag because the inner body has multiple early-returns.
  let order_status_byname_observed = false;
  try {
    tpccRetry(() => {
      order_status_byname_observed = false;
      driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
        let c_id: number;
        if (is_byname) {
          const cntRaw = tx.queryValue(
            sql("workload_tx_order_status", "count_customers_by_name")!,
            { w_id, d_id, c_last: c_last_pick },
          );
          const nameCount = Number(cntRaw ?? 0);
          if (nameCount === 0) return;  // nothing to report — same shape as by-id miss
          const offset = Math.floor((nameCount - 1) / 2);
          let nameRow: any[] | undefined;
          if (IS_PICODATA) {
            // picodata/sbroad rejects OFFSET — fetch all, pick median.
            const allRows = tx.queryRows(
              sql("workload_tx_order_status", "get_customer_by_name")!,
              { w_id, d_id, c_last: c_last_pick },
            );
            nameRow = allRows[offset];
          } else {
            nameRow = tx.queryRow(
              sql("workload_tx_order_status", "get_customer_by_name")!,
              { w_id, d_id, c_last: c_last_pick, offset },
            );
          }
          if (!nameRow) return;
          // By-name SELECT returns [c_balance, c_first, c_middle, c_last, c_id]
          // — c_id is the last column.
          c_id = Number(nameRow[nameRow.length - 1]);
          order_status_byname_observed = true;
        } else {
          c_id = c_id_pick;
          const custRow = tx.queryRow(
            sql("workload_tx_order_status", "get_customer_by_id")!, { c_id, d_id, w_id },
          );
          if (!custRow) return;
        }

        // Spec §2.6.2.2: find the customer's most-recent order.
        const lastRow = tx.queryRow(
          sql("workload_tx_order_status", "get_last_order")!, { d_id, w_id, c_id },
        );
        if (!lastRow) return;  // customer has no orders yet
        const o_id = Number(lastRow[0]);

        // Spec §2.6.2.2: read order lines for that order.
        tx.queryRows(sql("workload_tx_order_status", "get_order_lines")!, { o_id, d_id, w_id });
      });
    });
    if (order_status_byname_observed) tpccOrderStatusByname.add(1);
  } finally {
    tpccOrderStatusDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// DELIVERY (4% of mix) — loops over all districts in a warehouse.
// Spec §2.7: for each district, find MIN(no_o_id) → delete that new_order
// row → read orders.o_c_id for the order → update carrier → update
// ol_delivery_d → sum ol_amount → update customer balance by that sum.
// Every ID and amount used below comes from a real SELECT inside the tx.
//   - §2.7.1.1: w_id is the terminal's fixed home warehouse.
// =====================================================================
const deliveryOCarrierIdGen = DrawRT.intUniform(seedOf("delivery.o_carrier_id"), 1, 10);

function delivery() {
  tpccDeliveryTotal.add(1);
  const t0 = Date.now();
  const w_id       = HOME_W_ID;
  const carrier_id = deliveryOCarrierIdGen.next();

  // T2.3: Delivery is much larger than NO/P (10 districts × ~6 statements
  // each), so a 40001 retry compounds the round-trip cost. Spec §5.2.5.4
  // gives Delivery an 80s p90 ceiling — plenty of headroom — and 40001
  // remains rare here in practice (Delivery's MIN(no_o_id) + DELETE pattern
  // doesn't hot-row with concurrent NO bumps). Use the same retry budget
  // anyway so the fail mode is uniform across tx types.
  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
        for (let d_id = 1; d_id <= DISTRICTS_PER_WAREHOUSE; d_id++) {
          const minRow = tx.queryRow(
            sql("workload_tx_delivery", "get_min_new_order")!, { d_id, w_id },
          );
          if (!minRow || minRow[0] === null || minRow[0] === undefined) {
            continue;  // nothing to deliver in this district
          }
          const o_id = Number(minRow[0]);

          tx.exec(sql("workload_tx_delivery", "delete_new_order")!, { o_id, d_id, w_id });

          const orderRow = tx.queryRow(
            sql("workload_tx_delivery", "get_order")!, { o_id, d_id, w_id },
          );
          if (!orderRow) continue;
          const c_id = Number(orderRow[0]);

          tx.exec(sql("workload_tx_delivery", "update_order")!, { carrier_id, o_id, d_id, w_id });
          tx.exec(sql("workload_tx_delivery", "update_order_line")!, { o_id, d_id, w_id });

          const sumRow = tx.queryRow(
            sql("workload_tx_delivery", "get_order_line_amount")!, { o_id, d_id, w_id },
          );
          const ol_total = sumRow && sumRow[0] !== null ? Number(sumRow[0]) : 0;

          tx.exec(sql("workload_tx_delivery", "update_customer")!, {
            amount: ol_total, c_id, d_id, w_id,
          });
        }
      });
    });
  } finally {
    tpccDeliveryDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// STOCK_LEVEL (4% of mix) — read-only.
// Spec §2.8: read district's d_next_o_id → count distinct stock items
// in the last 20 orders of that district whose stock is below threshold.
// The scan window's upper bound comes from the district SELECT, not a
// random number.
//   - §2.8.1.1: w_id is the terminal's fixed home warehouse; d_id is
//               uniform over the 10 districts. (The spec actually pins
//               d_id per terminal too, but uniform is closer to the
//               populated-clients case.)
// =====================================================================
const slevDIdGen       = DrawRT.intUniform(seedOf("slev.d_id"), 1, DISTRICTS_PER_WAREHOUSE);
const slevThresholdGen = DrawRT.intUniform(seedOf("slev.threshold"), 10, 20);

function stock_level() {
  tpccStockLevelTotal.add(1);
  const t0 = Date.now();
  const w_id      = HOME_W_ID;
  const d_id      = slevDIdGen.next();
  const threshold = slevThresholdGen.next();

  // T2.3: Stock-Level is read-only and rarely 40001s, but the retry wrap
  // is cheap and keeps the dispatch shape uniform across all five tx types.
  try {
    tpccRetry(() => {
      driver.beginTx({ isolation: TX_ISOLATION }, (tx) => {
        const next_o_id = tx.queryValue<number>(
          sql("workload_tx_stock_level", "get_district")!, { w_id, d_id },
        );
        if (next_o_id === undefined) return;

        // Two-step scan. The obvious single-query JOIN/subquery form trips
        // picodata's sbroad planner intermittently ("Temporary SQL table
        // TMP_... not found"), so we collect the window's distinct item ids
        // first and count low-stock rows against an inline IN list.
        const olRows = tx.queryRows(
          sql("workload_tx_stock_level", "get_window_items")!,
          { w_id, d_id, min_o_id: next_o_id - 20, next_o_id },
        );
        if (olRows.length === 0) return;

        const ids: number[] = [];
        for (let i = 0; i < olRows.length; i++) {
          const v = Number(olRows[i][0]);
          if (Number.isFinite(v)) ids.push(v);
        }
        if (ids.length === 0) return;

        // Inline the integer list; stroppy's :name substitution leaves IN list
        // contents alone and the ids come from a trusted SELECT, not user input.
        const template = sql("workload_tx_stock_level", "stock_count_in")!;
        const rendered = template.sql.replace("{ids}", ids.join(","));
        tx.queryValue<number>(rendered, { w_id, threshold });
      });
    });
  } finally {
    tpccStockLevelDuration.add(Date.now() - t0);
  }
}

// =====================================================================
// Weighted dispatch — TPC-C standard mix: 45/43/4/4/4 (sums to 100)
// =====================================================================
const picker = NewPicker(0);

// §5.2.1/§5.2.5: each VU iteration is: keying time → tx → think time.
// Pacing is opt-in (PACING=true); default off for raw throughput runs.
const _txNameByFn = new Map<Function, string>([
  [new_order, "new_order"], [payment, "payment"], [order_status, "order_status"],
  [delivery, "delivery"], [stock_level, "stock_level"],
]);
// STROPPY_NO_DEFAULT=1 short-circuits the default() iteration to a no-op.
// k6 always runs default() at least once (minimum 1 VU × 1 iter); integration
// tests that only want to validate the load phase can set this env var to
// observe the post-populate state without any transaction mutations.
const NO_DEFAULT = ENV("STROPPY_NO_DEFAULT", "false", "Skip the transaction body in default()") === "true";

export default function (): void {
  if (NO_DEFAULT) return;
  const workload = picker.pickWeighted(
    [new_order, payment, order_status, delivery, stock_level],
    [45, 43, 4, 4, 4],
  ) as () => void;
  const txName = _txNameByFn.get(workload) ?? "new_order";
  keyingTime(txName);
  workload();
  thinkTime(txName);
}

export function teardown() {
  Step.end("workload");
  Teardown();
}

// =====================================================================
// handleSummary — TPC-C §1.11 post-run transaction mix + compliance rates.
// Overrides the default k6 end-of-test summary. Prints observed percentages
// alongside spec bounds so operators can verify compliance without
// instrumenting the DB.
//
// T3.1: statistical assertion on spec §5.2.3 minimum mix (NO 45 / P 43 /
// OS 4 / D 4 / SL 4). We use a one-sided 3σ upper bound against the floor:
// flag only if `observed_share + 3*sqrt(p*(1-p)/N)*100 < floor`, i.e. if
// the true share is genuinely below spec at ~99.87% confidence. This
// replaces an earlier fixed 1pp tolerance that tripped on natural Bernoulli
// noise for the 4%-class types during 10-30s smoke runs (std ≈ 0.4pp at
// N=2000, ≈0.9pp at N=500). Sample gate is 50 txs — below that the normal
// approximation is unreliable.
//
// Violations are printed inline in stdout, NOT thrown. A thrown
// handleSummary causes k6 to discard the custom output and fall back to
// its default summary — burying exactly the data the operator needs to
// diagnose the violation. k6 threshold failures (p90 ceilings on the
// tpcc_*_duration Trends via `options.thresholds` above) still mark the
// run as failed in the k6 exit code, so real compliance gates remain.
//
// T3.2: per-tx full-distribution (avg/p50/p90/p95/p99) is printed so
// operators can see the shape of the response-time distribution without
// shelling out to the raw metrics. The spec ceiling is in the same line
// for quick visual comparison.
//
// Driver-layer section surfaces helpers.ts metrics (run_query_duration,
// tx_total_duration, etc.) so the operator gets a full per-run picture in
// one place and doesn't have to re-run with a different summary format.
// =====================================================================
/* eslint-disable @typescript-eslint/no-explicit-any */
export function handleSummary(data: any): Record<string, string> {
  const m = data.metrics ?? {};
  const cnt = (name: string): number => Number(m[name]?.values?.count ?? 0);
  const pct = (num: number, den: number): string =>
    den > 0 ? ((num / den) * 100).toFixed(2) + "%" : "n/a";

  // Pretty full-percentile trend line. Covers both the TPC-C per-tx trends
  // (time_unit: true → ms) and the driver-layer trends. Non-numeric fields
  // render as "—" so missing metrics degrade visibly instead of crashing.
  const trendLine = (name: string): string => {
    const v = m[name]?.values ?? {};
    const fmt = (x: any) => (typeof x === "number" ? x.toFixed(1) : "—");
    return `avg=${fmt(v.avg)}  p50=${fmt(v.med)}  p90=${fmt(v["p(90)"])}  p95=${fmt(v["p(95)"])}  p99=${fmt(v["p(99)"])}`;
  };
  const rateStr = (name: string): string => {
    const v = m[name]?.values?.rate;
    return typeof v === "number" ? (v * 100).toFixed(2) + "%" : "n/a";
  };

  const no  = cnt("tpcc_new_order_total");
  const pay = cnt("tpcc_payment_total");
  const os  = cnt("tpcc_order_status_total");
  const dl  = cnt("tpcc_delivery_total");
  const sl  = cnt("tpcc_stock_level_total");
  const tot = no + pay + os + dl + sl;

  const rbDone = cnt("tpcc_rollback_done");
  const rlTot  = cnt("tpcc_remote_line_total");
  const rlRem  = cnt("tpcc_remote_line_remote");
  const payRem = cnt("tpcc_payment_remote");
  const payBN  = cnt("tpcc_payment_byname");
  const payBC  = cnt("tpcc_payment_bc");
  const osBN   = cnt("tpcc_order_status_byname");
  const retries = cnt("tpcc_retry_attempts");

  const iters   = cnt("iterations");
  const iterDur = m.iteration_duration?.values?.avg;
  const iterDurStr = typeof iterDur === "number" ? iterDur.toFixed(2) + " ms" : "n/a";
  const queries = cnt("run_query_count");

  const lines: string[] = [
    "",
    "===== TPC-C transaction mix (observed vs spec §5.2.3) =====",
    `  new_order    : ${pct(no, tot).padStart(7)}  (spec 45%, min 45%)`,
    `  payment      : ${pct(pay, tot).padStart(7)}  (spec 43%, min 43%)`,
    `  order_status : ${pct(os, tot).padStart(7)}  (spec  4%, min  4%)`,
    `  delivery     : ${pct(dl, tot).padStart(7)}  (spec  4%, min  4%)`,
    `  stock_level  : ${pct(sl, tot).padStart(7)}  (spec  4%, min  4%)`,
    "",
    "===== TPC-C compliance rates =====",
    `  rollback rate          : ${pct(rbDone, no).padStart(7)}  (spec ~1% of new_order, §2.4.1.4)`,
    `  payment remote         : ${pct(payRem, pay).padStart(7)}  (spec  15% of payment,  §2.5.1.2)`,
    `  payment by-name        : ${pct(payBN, pay).padStart(7)}  (spec  60% of payment,  §2.5.1.2)`,
    `  payment BC credit      : ${pct(payBC, pay).padStart(7)}  (spec  10% of payment,  §2.5.2.2)`,
    `  order_status by-name   : ${pct(osBN, os).padStart(7)}  (spec  60% of order_status, §2.6.1.2)`,
    `  new_order remote lines : ${pct(rlRem, rlTot).padStart(7)}  (spec  ~1% of lines,  §2.4.1.5)`,
    // T2.3: serialization-retry stats. Numerator is retry attempts, not
    // distinct retried txs (each tx may retry up to RETRY_ATTEMPTS-1 times).
    // A non-zero value indicates contention is firing 40001 / deadlock and
    // the helper is reclaiming throughput before it counts against
    // tx_error_rate (spec §5.2.5 1% cap).
    `  serialization retries  : ${String(retries).padStart(7)}  (T2.3 retry helper, spec §5.2.5 / §4.1)`,
    "",
    "===== TPC-C per-tx response time distribution (ms; §5.2.5.4 p90 ceilings) =====",
    `  new_order    (ceil  5000): ${trendLine("tpcc_new_order_duration")}`,
    `  payment      (ceil  5000): ${trendLine("tpcc_payment_duration")}`,
    `  order_status (ceil  5000): ${trendLine("tpcc_order_status_duration")}`,
    `  stock_level  (ceil 20000): ${trendLine("tpcc_stock_level_duration")}`,
    `  delivery     (ceil 80000): ${trendLine("tpcc_delivery_duration")}`,
    "",
    "===== Driver query / tx timings (from helpers.ts metrics) =====",
    `  queries executed    : ${queries}`,
    `  run_query_duration  : ${trendLine("run_query_duration")}`,
    `  run_query_error_rate: ${rateStr("run_query_error_rate")}`,
    `  tx_total_duration   : ${trendLine("tx_total_duration")}`,
    `  tx_clean_duration   : ${trendLine("tx_clean_duration")}`,
    `  tx_queries_per_tx   : ${trendLine("tx_queries_per_tx")}`,
    `  tx_commit_rate      : ${rateStr("tx_commit_rate")}`,
    `  tx_error_rate       : ${rateStr("tx_error_rate")}`,
    "",
    "===== k6 rollups =====",
    `  iterations        : ${iters}`,
    `  avg iter duration : ${iterDurStr}`,
    `  total tpcc txs    : ${tot}`,
    "",
  ];

  // Statistical mix-floor check — see the function-level comment for why
  // we use a 3σ one-sided bound instead of a fixed tolerance.
  const violations: string[] = [];
  if (tot >= 50) {
    const check = (label: string, got: number, floor: number) => {
      const p = got / tot;
      const share = p * 100;
      const stdPct = Math.sqrt((p * (1 - p)) / tot) * 100;
      const upper = share + 3 * stdPct;
      if (upper < floor) {
        violations.push(
          `  ${label.padEnd(13)}: observed ${share.toFixed(2)}% ±${stdPct.toFixed(2)}pp, ` +
          `3σ upper bound ${upper.toFixed(2)}% < floor ${floor}% (spec §5.2.3, N=${tot})`,
        );
      }
    };
    check("new_order",    no,  45);
    check("payment",      pay, 43);
    check("order_status", os,  4);
    check("delivery",     dl,  4);
    check("stock_level",  sl,  4);
  } else {
    lines.push(
      `(T3.1: skipping mix floor check — total ${tot} < 50, insufficient sample)`,
      "",
    );
  }

  if (violations.length > 0) {
    lines.push(
      "===== TPC-C §5.2.3 mix floor VIOLATIONS =====",
      ...violations,
      "",
      "(Violation means 3σ upper bound still below the spec floor — i.e. the",
      " picker is genuinely miscalibrated, not just sampling noise. The k6",
      " run exit code is determined by options.thresholds, not this check.)",
      "",
    );
  }

  return { stdout: lines.join("\n") };
}
/* eslint-enable @typescript-eslint/no-explicit-any */
