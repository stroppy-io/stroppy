import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, Step, ENV, TxIsolationName, declareDriverSetup } from "./helpers.ts";
import {
  Alphabet,
  Attr,
  Deg,
  Dict,
  Draw,
  Expr,
  InsertMethod as DatagenInsertMethod,
  Rel,
  RowIndex_Kind,
  Strat,
  std,
  type DictBody,
} from "./datagen.ts";
import { parse_sql_with_sections } from "./parse_sql.js";
// Note: `import ... from "./distributions.json" with { type: "json" }`
// hangs k6's goja runtime at module-compile (v1.7.0). Load the same blob
// via `open()` instead — same content, instant startup. Wrapped in a
// tolerant parse so the probe phase (where `open()` is stubbed to "") sees
// an empty-but-structurally-valid payload; setup() replaces it lazily.
function readDistributions(): TpchDistributions {
  const raw = open("./distributions.json");
  if (!raw) return { version: "", source: "", distributions: {} };
  return JSON.parse(raw) as TpchDistributions;
}
const distributions: TpchDistributions = readDistributions();
import {
  makeTpchText,
  makeTpchGrammarDicts,
  tpchPhone,
  tpchRetailPrice,
  tpchMfgrId,
  tpchMfgr,
  tpchBrand,
  tpchClerk,
  tpchOrderkey,
  tpchDateToDays,
  tpchDaysToDate,
  tpchOrderdateExpr,
  type TpchDistributions,
} from "./tpch_helpers.ts";
import {
  runAndCompareAllQueries,
  logSummary,
  type AnswersFile,
} from "./tpch_validate.ts";

// ============================================================================
// Data-gen simplifications (framework capability proof, not dbgen byte-parity):
//
//   1. Flat populations with row-index-derived keys for region / nation /
//      part / supplier / partsupp / customer.
//   2. part ↔ partsupp is expressed with deterministic row math so each
//      part has exactly four distinct suppkeys.
//   3. n_name / n_regionkey are read from a pair of scalar dicts built
//      from distributions.nations; n_regionkey follows dbgen's mapping
//      verbatim so q5 / q7 / q8 keep their expected regional shape.
//   4. Addresses and phones are ASCII draws (enSpc / enNumSpc / num
//      alphabets). Comment strings use the spec-compliant v-string
//      grammar walker (Draw.grammar) over the ten grammar / np / vp /
//      nouns / verbs / adjectives / adverbs / auxillaries /
//      prepositions / terminators dicts.
//
// Spec-faithful as of this file:
//   - o_orderkey is sparse (spec §4.2.3 / dbgen bm_utils.c): per 32
//     keys, 8 are kept and 24 skipped. Max key = 6_000_000 × SF.
//   - orders ↔ lineitem uses Relationship { orders Fixed(1), lineitem
//     Uniform(1, 7) }. l_orderkey references orders via Lookup.
//   - l_shipdate / l_commitdate / l_receiptdate are derived from
//     o_orderdate with uniform per-line offsets (spec §4.2.3).
//   - l_extendedprice = p_retailprice × l_quantity via Lookup into
//     part. l_discount uniform [0, 0.10]; l_tax uniform [0, 0.08].
//   - o_totalprice is recomputed from lineitems by a post-load UPDATE
//     (`finalize_totals` step), since the spec's formula depends on
//     yet-to-be-generated lineitems at orders-emit time.
//
// With the grammar-based tpchText, q13's `o_comment NOT LIKE
// '%special%requests%'` sees real word co-occurrences and its match
// distribution tracks dbgen closely. Q9's `p_name LIKE '%green%'`
// reads p_name, which is still a `Draw.phrase` over the colors vocab.
// Byte-exact dbgen parity stays a later follow-up; what ships here is
// grammatical shape faithful to the spec.
// ============================================================================

// --------------------------------------------------------------------------
// Configuration
// --------------------------------------------------------------------------

const POOL_SIZE = ENV("POOL_SIZE", 50, "Connection pool size");
const SCALE_FACTOR = Number(
  ENV("SCALE_FACTOR", "1", "TPC-H scale factor; 0.01 supported for smoke tests"),
);

if (!Number.isFinite(SCALE_FACTOR) || SCALE_FACTOR <= 0) {
  throw new Error(`SCALE_FACTOR must be a positive number, got ${SCALE_FACTOR}`);
}

/** Round SF-scaled cardinalities up to at least 1 row. */
function scaleRows(base: number): number {
  const n = Math.floor(base * SCALE_FACTOR);
  return n < 1 ? 1 : n;
}

// Spec §4.2.2 cardinalities.
const N_REGION = 5;
const N_NATION = 25;
const N_PART = scaleRows(200_000);
const N_SUPPLIER = scaleRows(10_000);
const N_CUSTOMER = scaleRows(150_000);
const N_ORDERS = scaleRows(1_500_000);
const N_CLERKS = scaleRows(1_000);
const PARTSUPPS_PER_PART = 4;
const N_PARTSUPP = N_PART * PARTSUPPS_PER_PART;
// Spec §4.2.3: each order has Uniform(1, 7) line items — mean 4 per
// order. The runtime computes the actual total from the degree draw;
// this constant is kept for the `Rel.table.size` hint on lineitem, which
// the relationship runtime overrides with the real cumulative sum.
const LINES_PER_ORDER_MIN = 1;
const LINES_PER_ORDER_MAX = 7;
const N_LINEITEM_EST = N_ORDERS * 4;

// Per-line date offset bands (spec §4.2.3).
const L_SHIPDATE_OFF_MIN = 1;
const L_SHIPDATE_OFF_MAX = 121;
const L_COMMITDATE_OFF_MIN = 30;
const L_COMMITDATE_OFF_MAX = 90;
const L_RECEIPTDATE_OFF_MIN = 1;
const L_RECEIPTDATE_OFF_MAX = 30;

// Spec-frozen per-population seeds.
const SEED_REGION = 0x7EC101;
const SEED_NATION = 0x7EC102;
const SEED_PART = 0x7EC103;
const SEED_SUPPLIER = 0x7EC104;
const SEED_PARTSUPP = 0x7EC105;
const SEED_CUSTOMER = 0x7EC106;
const SEED_ORDERS = 0x7EC107;
const SEED_LINEITEM = 0x7EC108;

// Date windows live in tpch_helpers.ts (tpchOrderdateExpr). Lineitem
// dates are derived from o_orderdate — see lineitemSpec().

export const options: Options = {
  setupTimeout: String(Math.max(5, Math.ceil(SCALE_FACTOR * 15))) + "m",
};

// --------------------------------------------------------------------------
// Driver / SQL wiring
// --------------------------------------------------------------------------

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
const SQL_FILE =
  ENV("SQL_FILE", ENV.auto, "SQL file path (defaults per driverType)") ??
  _sqlByDriver[driverConfig.driverType!] ??
  "./pg.sql";

// YDB declares currency columns as `Double` — unlike pg/mysql/pico which
// accept int64 into DECIMAL. Framework emits float64 from Draw.decimal,
// but Expr.lit(0.0) collapses to int64 on the wire (Number.isInteger(0.0)
// is true in JS). `Expr.litFloat(...)` forces the Double oneofKind so the
// zero-init placeholder for o_totalprice serializes as Double on YDB;
// pg/mysql/pico accept it identically into their DECIMAL/NUMERIC columns.

const _isoByDriver: Record<string, TxIsolationName> = {
  postgres: "read_committed",
  mysql: "read_committed",
  picodata: "none",
  ydb: "serializable",
};
const TX_ISOLATION = (
  ENV("TX_ISOLATION", ENV.auto, "Override transaction isolation level") ??
  _isoByDriver[driverConfig.driverType!] ??
  "read_committed"
) as TxIsolationName;
void TX_ISOLATION; // queries are read-only; kept for symmetry with other workloads.

const driver = DriverX.create().setup(driverConfig);
const sql = parse_sql_with_sections(open(SQL_FILE));

// --------------------------------------------------------------------------
// Dict wiring — pulled from distributions.json
// --------------------------------------------------------------------------

/**
 * Build a scalar Dict from a distributions.json entry's `value` column.
 * Ignores weights — draws from these dicts are uniform.
 *
 * Tolerates an empty distributions map: the probe phase stubs `open()` to
 * return "", producing a payload with no dicts. In that case we emit a
 * single-entry placeholder dict; probe-time dict content is never read.
 */
function scalarDictFromJson(name: string): DictBody {
  const d = distributions.distributions[name];
  if (!d || d.rows.length === 0) {
    return Dict.values([""]);
  }
  const values = d.rows.map((r) => String(r.values[0]));
  return Dict.values(values);
}

const regionsDict = scalarDictFromJson("regions");
const nationsNameDict = scalarDictFromJson("nations");
// Nation→region mapping from dbgen's cumulative-weight walk over
// distributions.nations (spec §4.2.3). Stable constants kept inline so we
// don't reinterpret the signed weights inside distributions.json.
const nationRegionKeys: readonly number[] = [
  0, 1, 1, 1, 4, 0, 3, 3, 2, 2, 4, 4, 2, 4, 0, 0, 0, 1, 2, 3, 4, 2, 3, 3, 1,
];
if (nationRegionKeys.length !== N_NATION) {
  throw new Error(`tpch: nationRegionKeys length ${nationRegionKeys.length} != ${N_NATION}`);
}

// Dict.values always stringifies its entries (DictRow.values is string on the
// wire), so we coerce back to int64 via Attr.dictAtInt at read time. YDB's
// BulkUpsert requires an Int64 for n_regionkey; pg/mysql/pico accept either.
const nationRegionKeyDict = Dict.values(nationRegionKeys.map(String));
const mktSegmentDict = scalarDictFromJson("msegmnt");
const orderPriorityDict = scalarDictFromJson("o_oprio");
const containerDict = scalarDictFromJson("p_cntr");
const typesDict = scalarDictFromJson("p_types");
const shipInstructDict = scalarDictFromJson("instruct");
const shipModeDict = scalarDictFromJson("smode");
const returnFlagDict = scalarDictFromJson("rflag");
const colorsDict = scalarDictFromJson("colors");
const linestatusDict = Dict.values(["O", "F"]); // simplified l_linestatus

// Grammar dict bundle + the v-string helper bound to them. Dicts carry
// their native weights (distributions.json uses weight_sets=["default"])
// so the evaluator honors spec-§4.2.2 word frequencies.
const tpchGrammarDicts = makeTpchGrammarDicts(distributions.distributions);
const tpchText = makeTpchText(tpchGrammarDicts);

// --------------------------------------------------------------------------
// Shared sub-expressions
// --------------------------------------------------------------------------

/** Zero-padded 9-digit id — "%09d" — used by Supplier# / Customer# names. */
function fmt9(id: ReturnType<typeof Attr.rowId>) {
  return std.format(Expr.lit("%09d"), id);
}

// --------------------------------------------------------------------------
// Per-table InsertSpec builders
// --------------------------------------------------------------------------

function regionSpec() {
  return Rel.table("region", {
    size: N_REGION,
    seed: SEED_REGION,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      r_regionkey: Attr.rowIndex(),
      r_name: Attr.dictAt(regionsDict, Attr.rowIndex()),
      r_comment: tpchText(31, 115),
    },
  });
}

function nationSpec() {
  return Rel.table("nation", {
    size: N_NATION,
    seed: SEED_NATION,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      n_nationkey: Attr.rowIndex(),
      n_name: Attr.dictAt(nationsNameDict, Attr.rowIndex()),
      n_regionkey: Attr.dictAtInt(nationRegionKeyDict, Attr.rowIndex()),
      n_comment: tpchText(31, 114),
    },
  });
}

function partSpec() {
  const mfgrId = tpchMfgrId();
  return Rel.table("part", {
    size: N_PART,
    seed: SEED_PART,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      p_partkey: Attr.rowId(),
      p_name: Draw.phrase({
        vocab: colorsDict,
        minWords: Expr.lit(5),
        maxWords: Expr.lit(5),
        separator: " ",
      }),
      p_mfgr: tpchMfgr(mfgrId),
      p_brand: tpchBrand(mfgrId),
      p_type: Draw.dict(typesDict),
      p_size: Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(50) }),
      p_container: Draw.dict(containerDict),
      p_retailprice: tpchRetailPrice(Attr.rowId()),
      p_comment: tpchText(5, 22),
    },
  });
}

function supplierSpec() {
  return Rel.table("supplier", {
    size: N_SUPPLIER,
    seed: SEED_SUPPLIER,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      s_suppkey: Attr.rowId(),
      s_name: Expr.concat(Expr.lit("Supplier#"), fmt9(Attr.rowId())),
      s_address: Draw.ascii({
        min: Expr.lit(25),
        max: Expr.lit(40),
        alphabet: Alphabet.enNumSpc,
      }),
      s_nationkey: Draw.intUniform({ min: Expr.lit(0), max: Expr.lit(N_NATION - 1) }),
      s_phone: tpchPhone(
        Draw.intUniform({ min: Expr.lit(0), max: Expr.lit(N_NATION - 1) }),
      ),
      s_acctbal: Draw.decimal({ min: Expr.lit(-999.99), max: Expr.lit(9999.99), scale: 2 }),
      s_comment: tpchText(25, 100),
    },
  });
}

function partSuppSpec() {
  // Flat row-math layout:
  //   r ∈ [0, 4 * N_PART)
  //   ps_partkey = r / 4 + 1 ∈ [1, N_PART]
  //   ps_suppkey = wrap((partkey + stride * (r % 4)) mod N_SUPPLIER) + 1
  // Stride = floor(N_SUPPLIER / 4) gives four distinct suppkeys per part
  // while keeping the choice deterministic by row index (seek-safe).
  const stride = Math.max(1, Math.floor(N_SUPPLIER / 4));
  const partkey = Expr.add(Expr.div(Attr.rowIndex(), Expr.lit(PARTSUPPS_PER_PART)), Expr.lit(1));
  const offset = Expr.mod(Attr.rowIndex(), Expr.lit(PARTSUPPS_PER_PART));
  const suppkey = Expr.add(
    Expr.mod(
      Expr.add(partkey, Expr.mul(offset, Expr.lit(stride))),
      Expr.lit(N_SUPPLIER),
    ),
    Expr.lit(1),
  );
  return Rel.table("partsupp", {
    size: N_PARTSUPP,
    seed: SEED_PARTSUPP,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      ps_partkey: partkey,
      ps_suppkey: suppkey,
      ps_availqty: Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(9999) }),
      ps_supplycost: Draw.decimal({ min: Expr.lit(1.0), max: Expr.lit(1000.0), scale: 2 }),
      ps_comment: tpchText(49, 198),
    },
  });
}

function customerSpec() {
  return Rel.table("customer", {
    size: N_CUSTOMER,
    seed: SEED_CUSTOMER,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      c_custkey: Attr.rowId(),
      c_name: Expr.concat(Expr.lit("Customer#"), fmt9(Attr.rowId())),
      c_address: Draw.ascii({
        min: Expr.lit(10),
        max: Expr.lit(40),
        alphabet: Alphabet.enNumSpc,
      }),
      c_nationkey: Draw.intUniform({ min: Expr.lit(0), max: Expr.lit(N_NATION - 1) }),
      c_phone: tpchPhone(
        Draw.intUniform({ min: Expr.lit(0), max: Expr.lit(N_NATION - 1) }),
      ),
      c_acctbal: Draw.decimal({ min: Expr.lit(-999.99), max: Expr.lit(9999.99), scale: 2 }),
      c_mktsegment: Draw.dict(mktSegmentDict),
      c_comment: tpchText(29, 116),
    },
  });
}

function ordersSpec() {
  return Rel.table("orders", {
    size: N_ORDERS,
    seed: SEED_ORDERS,
    method: DatagenInsertMethod.NATIVE,
    attrs: {
      // Sparse orderkey per spec §4.2.3 / dbgen bm_utils.c; see
      // tpchOrderkey() for the formula. The lineitem spec derives
      // l_orderkey from the same formula via an orders LookupPop.
      o_orderkey: tpchOrderkey(Attr.rowIndex()),
      o_custkey: Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(N_CUSTOMER) }),
      // Simplified 'O'/'F' split; 'P' (partial) omitted. Q21 only filters 'F'.
      // Bernoulli evaluates to int64 {0,1}; Expr.if expects a bool cond,
      // so lift with an explicit equality check.
      o_orderstatus: Expr.if(
        Expr.eq(Draw.bernoulli({ p: 0.5 }), Expr.lit(1)),
        Expr.lit("F"),
        Expr.lit("O"),
      ),
      // Placeholder — filled in by the finalize_totals SQL step as
      //   o_totalprice = Σ l_extendedprice × (1 + l_tax) × (1 - l_discount)
      // across matching lineitems (spec §4.2.3). Can't be computed at
      // orders-emit time because it depends on not-yet-generated lines.
      // Expr.litFloat keeps YDB's Double wire happy; pg/mysql/pico accept
      // it identically into their DECIMAL/NUMERIC columns.
      o_totalprice: Expr.litFloat(0.0),
      // Deterministic per-row orderdate (hash(rowIndex) mod 2557); same
      // formula is exposed via the lineitem orders LookupPop so
      // lineitem's derived dates reference the exact stored value.
      o_orderdate: tpchOrderdateExpr(Attr.rowIndex()),
      o_orderpriority: Draw.dict(orderPriorityDict),
      o_clerk: tpchClerk(N_CLERKS),
      o_shippriority: Expr.lit(0),
      o_comment: tpchText(19, 78),
    },
  });
}

function lineitemSpec() {
  // Spec §4.2.3: each order carries Uniform(1, 7) line items. Lineitem
  // is iterated over an outer orders LookupPop via a 2-side
  // Relationship; the runtime resolves the true total from the
  // per-entity degree draw. The `size` hint we pass to Rel.table is
  // overridden once the relationship is installed.
  //
  // LookupPop layout:
  //   orders (outer) — replays o_orderkey / o_orderdate using the
  //     same formulas as ordersSpec() so Lookup reads round-trip.
  //   part (sibling) — exposes p_retailprice keyed by partkey-1;
  //     lineitem reads it to derive l_extendedprice.
  const ordersLookup = Rel.lookupPop({
    name: "orders",
    size: N_ORDERS,
    attrs: {
      o_orderkey: tpchOrderkey(Attr.rowIndex()),
      // Must mirror ordersSpec().o_orderdate exactly: both live in
      // different evaluation contexts (different rootSeed, different
      // attrPath) so any Draw.* would diverge. A pure hash-mod keeps
      // the formula-driven date identical across contexts.
      o_orderdate: tpchOrderdateExpr(Attr.rowIndex()),
    },
  });
  const partLookup = Rel.lookupPop({
    name: "part",
    size: N_PART,
    attrs: {
      // p_partkey is 1-based rowId in partSpec; we expose it here so the
      // lookup `part.p_retailprice` at entity index (l_partkey - 1)
      // returns the retailprice of the part keyed by l_partkey.
      p_retailprice: tpchRetailPrice(Attr.rowId()),
    },
  });

  const entityIdx = Attr.rowIndex(RowIndex_Kind.ENTITY);

  // Stream draws are seeded by (root, attr_path, stream_id, row_idx), so
  // the same Draw.* expression re-evaluated under two different attr
  // paths returns two different values. To keep spec invariants
  // (l_extendedprice = p_retailprice × l_quantity, l_receiptdate >
  // l_shipdate > o_orderdate) we materialize each random component into
  // its own attr and reference it through Expr.col() from downstream
  // attrs. Attr evaluation follows declaration order in the DAG.

  const ordersSide = Rel.side("orders", {
    degree: Deg.fixed(1),
    strategy: Strat.sequential(),
  });
  const lineitemSide = Rel.side("lineitem", {
    degree: Deg.uniform(LINES_PER_ORDER_MIN, LINES_PER_ORDER_MAX),
    strategy: Strat.sequential(),
  });

  return Rel.table("lineitem", {
    size: N_LINEITEM_EST,
    seed: SEED_LINEITEM,
    method: DatagenInsertMethod.NATIVE,
    lookupPops: [ordersLookup, partLookup],
    relationships: [Rel.relationship("orders_lineitem", [ordersSide, lineitemSide])],
    iter: "orders_lineitem",
    attrs: {
      l_orderkey: Attr.lookup("orders", "o_orderkey", entityIdx),
      l_partkey: Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(N_PART) }),
      l_suppkey: Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(N_SUPPLIER) }),
      l_linenumber: Expr.add(Attr.rowIndex(RowIndex_Kind.LINE), Expr.lit(1)),
      l_quantity: Draw.decimal({ min: Expr.lit(1.0), max: Expr.lit(50.0), scale: 0 }),
      l_extendedprice: Expr.mul(
        Attr.lookup("part", "p_retailprice", Expr.sub(Expr.col("l_partkey"), Expr.lit(1))),
        Expr.col("l_quantity"),
      ),
      l_discount: Draw.decimal({ min: Expr.lit(0.0), max: Expr.lit(0.1), scale: 2 }),
      l_tax: Draw.decimal({ min: Expr.lit(0.0), max: Expr.lit(0.08), scale: 2 }),
      l_returnflag: Draw.dict(returnFlagDict),
      l_linestatus: Draw.dict(linestatusDict),
      l_shipdate: tpchDaysToDate(
        Expr.add(
          tpchDateToDays(Attr.lookup("orders", "o_orderdate", entityIdx)),
          Draw.intUniform({
            min: Expr.lit(L_SHIPDATE_OFF_MIN),
            max: Expr.lit(L_SHIPDATE_OFF_MAX),
          }),
        ),
      ),
      l_commitdate: tpchDaysToDate(
        Expr.add(
          tpchDateToDays(Attr.lookup("orders", "o_orderdate", entityIdx)),
          Draw.intUniform({
            min: Expr.lit(L_COMMITDATE_OFF_MIN),
            max: Expr.lit(L_COMMITDATE_OFF_MAX),
          }),
        ),
      ),
      // Reads the already-computed l_shipdate via Expr.col so the
      // receipt offset is added to the exact same shipdate that landed
      // in the row. Lookup + DateToDays are cheap (LookupPop has an LRU
      // and std.dateToDays is pure) so repeating the orderdate read
      // here doesn't change behaviour.
      l_receiptdate: tpchDaysToDate(
        Expr.add(
          tpchDateToDays(Expr.col("l_shipdate")),
          Draw.intUniform({
            min: Expr.lit(L_RECEIPTDATE_OFF_MIN),
            max: Expr.lit(L_RECEIPTDATE_OFF_MAX),
          }),
        ),
      ),
      l_shipinstruct: Draw.dict(shipInstructDict),
      l_shipmode: Draw.dict(shipModeDict),
      l_comment: tpchText(10, 43),
    },
  });
}

// --------------------------------------------------------------------------
// Query parameter defaults — TPC-H §2.4 pinned values.
// --------------------------------------------------------------------------

// YDB / picodata lack `date + interval 'N month/year'` as an expression;
// we shift the anchor dates client-side and pass :date_NN alongside :date
// on those dialects. pg/mysql compute the shift inside the query (pg via
// `:date::date + interval '3 months'`, mysql via `DATE_ADD(:date, INTERVAL
// 3 MONTH)`). See pico.sql / ydb.sql for the placeholders consumed per q.
const NEEDS_END_DATES =
  driverConfig.driverType === "picodata" || driverConfig.driverType === "ydb";

function shiftDate(iso: string, days: number, months: number, years: number): string {
  const d = new Date(iso + "T00:00:00Z");
  d.setUTCFullYear(d.getUTCFullYear() + years);
  d.setUTCMonth(d.getUTCMonth() + months);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** Merge `{date_1m, date_3m, date_1y}` derived from `date` when NEEDS_END_DATES. */
function withEndDates(p: Record<string, unknown>): Record<string, unknown> {
  if (!NEEDS_END_DATES) return p;
  const d = p.date;
  if (typeof d !== "string") return p;
  return {
    ...p,
    date_1m: shiftDate(d, 0, 1, 0),
    date_3m: shiftDate(d, 0, 3, 0),
    date_1y: shiftDate(d, 0, 0, 1),
  };
}

const queryParams: Record<string, Record<string, unknown>> = {
  q1: { delta: 90 },
  q2: { size: 15, type: "BRASS", region: "EUROPE" },
  q3: { segment: "BUILDING", date: "1995-03-15" },
  q4: { date: "1993-07-01" },
  q5: { region: "ASIA", date: "1994-01-01" },
  q6: { date: "1994-01-01", discount: 0.06, quantity: 24 },
  q7: { nation1: "FRANCE", nation2: "GERMANY" },
  q8: { region: "AMERICA", nation: "BRAZIL", type: "ECONOMY ANODIZED STEEL" },
  q9: { color: "green" },
  q10: { date: "1993-10-01" },
  q11: { nation: "GERMANY", fraction: 0.0001 / SCALE_FACTOR },
  q12: { shipmode1: "MAIL", shipmode2: "SHIP", date: "1994-01-01" },
  q13: { word1: "special", word2: "requests" },
  q14: { date: "1995-09-01" },
  q15: { date: "1996-01-01" },
  q16: {
    brand: "Brand#45",
    type_prefix: "MEDIUM POLISHED",
    s1: 49, s2: 14, s3: 23, s4: 45, s5: 19, s6: 3, s7: 36, s8: 9,
  },
  q17: { brand: "Brand#23", container: "MED BOX" },
  q18: { quantity: 300 },
  q19: { brand1: "Brand#12", brand2: "Brand#23", brand3: "Brand#34", q1: 1, q2: 10, q3: 20 },
  q20: { color: "forest", nation: "CANADA", date: "1994-01-01" },
  q21: { nation: "SAUDI ARABIA" },
  q22: { cc1: "13", cc2: "31", cc3: "23", cc4: "29", cc5: "30", cc6: "18", cc7: "17" },
};

// --------------------------------------------------------------------------
// k6 lifecycle
// --------------------------------------------------------------------------

/** Run every parsed query in `section`; noop if the section is missing. */
function runSection(section: string): void {
  const queries = sql(section);
  if (!queries) return;
  queries.forEach((q) => driver.exec(q, {}));
}

export function setup(): void {
  Step("drop_schema", () => {
    runSection("drop_schema");
  });

  Step("create_schema", () => {
    runSection("create_schema");
  });

  Step("load_data", () => {
    driver.insertSpec(regionSpec());
    driver.insertSpec(nationSpec());
    driver.insertSpec(partSpec());
    driver.insertSpec(supplierSpec());
    driver.insertSpec(partSuppSpec());
    driver.insertSpec(customerSpec());
    driver.insertSpec(ordersSpec());
    driver.insertSpec(lineitemSpec());
  });

  // pg-only: flip UNLOGGED → LOGGED and ANALYZE. Other dialects ship the
  // section empty (or missing), so runSection just noops.
  Step("set_logged", () => {
    runSection("set_logged");
  });

  Step("create_indexes", () => {
    runSection("create_indexes");
  });

  // Spec §4.2.3: o_totalprice = Σ l_extendedprice × (1+l_tax) × (1-l_discount)
  // over lineitems. We fill it post-load since it depends on yet-to-be
  // generated lines at orders-emit time. Runs after create_indexes so
  // the correlated subquery can use idx_lineitem_orderkey (pg/mysql/pico).
  Step("finalize_totals", () => {
    runSection("finalize_totals");
  });

  Step("queries", () => {
    // Run each query once with pinned defaults. Log timings; tolerate
    // missing bodies gracefully so incremental bring-up works.
    for (let i = 1; i <= 22; i++) {
      const name = "q" + String(i);
      const body = sql(name, "body");
      if (!body) {
        console.log(`[tpch] ${name}: skipped (no body in SQL file)`);
        continue;
      }
      const params = withEndDates(queryParams[name] ?? {});
      const t0 = Date.now();
      try {
        driver.queryRows(body, params);
        console.log(`[tpch] ${name}: ok in ${Date.now() - t0}ms`);
      } catch (e) {
        console.log(`[tpch] ${name}: error ${(e as Error)?.message ?? e}`);
      }
    }
  });

  Step("validate_answers", () => {
    if (Math.abs(SCALE_FACTOR - 1) > 1e-9) {
      console.log(
        `[tpch_validate] skipped: answers_sf1 is SF=1 only, current SCALE_FACTOR=${SCALE_FACTOR}`,
      );
      return;
    }
    if (driverConfig.driverType !== "postgres") {
      console.log(
        `[tpch_validate] skipped: answers_sf1 generated against postgres only; driverType=${driverConfig.driverType}`,
      );
      return;
    }
    const queries: Record<string, import("./helpers.ts").SqlQuery | undefined> = {};
    for (let i = 1; i <= 22; i++) {
      const name = "q" + String(i);
      const body = sql(name, "body");
      if (body) queries[name] = body;
    }
    // Load the 2 MB answers blob only when we actually need it.
    const answers = JSON.parse(open("./answers_sf1.json")) as AnswersFile;
    const results = runAndCompareAllQueries(driver, queries, queryParams, answers);
    logSummary(results);
  });

  Step.begin("workload");
}

export default function (): void {
  // TPC-H has no per-iteration transaction workload; loading + querying
  // runs entirely from setup().
}

export function teardown(): void {
  Step.end("workload");
  Teardown();
}
