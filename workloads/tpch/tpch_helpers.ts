/**
 * tpch_helpers.ts — TPC-H-specific attr composition helpers built entirely on
 * top of the generic datagen surface. Nothing here touches Go code: every
 * routine returns an `Expr` that the runtime already understands.
 *
 * This file is the designated home for anything whose name starts with `tpch`
 * (spec §4.2 v-string grammar, phone-number builder, price formula). Adding a
 * new workload-specific helper? Put it here, not in `internal/static/`.
 */
import {
  Dict,
  Draw,
  Expr,
  std,
  type Expression,
  type DictBody,
} from "./datagen.ts";

/**
 * TPC-H v-string grammar (spec §4.2.2.14). The evaluator walks a
 * sentence template picked from `grammar`, resolves phrase-level
 * nonterminals N/V through `np`/`vp` (one level of expansion), then
 * emits leaves from nouns/verbs/adjectives/adverbs/auxillaries/
 * prepositions/terminators. Walked strings are truncated to `maxLen`
 * characters; when the first walk is shorter than `minLen`, the
 * evaluator re-walks up to 8 times before accepting the last attempt.
 *
 * Correctness consequences:
 *  - q13's `o_comment NOT LIKE '%special%requests%'` operates on real
 *    grammatical text, so the answer-side match count matches dbgen.
 *  - q9 is unaffected (p_name still uses Draw.phrase over colors).
 */
export interface TpchGrammarDicts {
  root: DictBody;
  np: DictBody;
  vp: DictBody;
  nouns: DictBody;
  verbs: DictBody;
  adjectives: DictBody;
  adverbs: DictBody;
  auxillaries: DictBody;
  prepositions: DictBody;
  terminators: DictBody;
}

/** Mint a `tpchText(min, max)` helper bound to the grammar dicts. */
export function makeTpchText(g: TpchGrammarDicts): (min: number, max: number) => Expression {
  return function tpchText(minLen: number, maxLen: number): Expression {
    return Draw.grammar({
      rootDict: g.root,
      phrases: { N: g.np, V: g.vp },
      leaves: {
        N: g.nouns,
        V: g.verbs,
        J: g.adjectives,
        D: g.adverbs,
        X: g.auxillaries,
        P: g.prepositions,
        T: g.terminators,
      },
      minLen,
      maxLen,
    });
  };
}

/**
 * Build a `TpchGrammarDicts` from a `distributions.json`-shaped map. The
 * ten referenced dist names are spec-frozen (root "grammar", np, vp,
 * nouns, verbs, adjectives, adverbs, auxillaries, prepositions,
 * terminators). Each lookup returns a weighted `DictBody` — the
 * evaluator honors the first weight set declared on each dict.
 */
export function makeTpchGrammarDicts(
  dists: Record<string, { columns?: readonly string[]; weight_sets?: readonly string[];
    rows: ReadonlyArray<{ values: readonly (string | number)[]; weights?: readonly number[] }> }>,
): TpchGrammarDicts {
  const pick = (name: string): DictBody => {
    const d = dists[name];
    if (!d || !d.rows || d.rows.length === 0) {
      return Dict.values([""]);
    }
    return Dict.fromJson({
      columns: d.columns,
      weight_sets: d.weight_sets,
      rows: d.rows.map((r) => ({
        values: r.values,
        weights: r.weights,
      })),
    });
  };
  return {
    root: pick("grammar"),
    np: pick("np"),
    vp: pick("vp"),
    nouns: pick("nouns"),
    verbs: pick("verbs"),
    adjectives: pick("adjectives"),
    adverbs: pick("adverbs"),
    auxillaries: pick("auxillaries"),
    prepositions: pick("prepositions"),
    terminators: pick("terminators"),
  };
}

/**
 * TPC-H phone number (spec §4.2.3 Clause 4.2.3). Format:
 *   <cc>-<loc1>-<loc2>-<loc3>
 * where cc = nationKey + 10 (two digits), and each loc segment is
 * uniform in the advertised digit-width range. The formula matches
 * dbgen's `PHONE_FUNC`, which guarantees substring(phone,1,2) reads
 * back as a valid nationality code — load q22 relies on that invariant.
 */
export function tpchPhone(nationKey: Expression): Expression {
  const cc = Expr.add(nationKey, Expr.lit(10));
  const loc1 = Draw.intUniform({ min: Expr.lit(100), max: Expr.lit(999) });
  const loc2 = Draw.intUniform({ min: Expr.lit(100), max: Expr.lit(999) });
  const loc3 = Draw.intUniform({ min: Expr.lit(1000), max: Expr.lit(9999) });
  return std.format(
    Expr.lit("%02d-%03d-%03d-%04d"),
    cc,
    loc1,
    loc2,
    loc3,
  );
}

/**
 * TPC-H part retail price formula (spec §4.2.3):
 *   p_retailprice = (90_000 + ((p_partkey / 10) % 20_001) + 100 * (p_partkey % 1_000)) / 100
 * Yields a decimal in roughly [900.00, 2099.00], always fixed-point with
 * two fractional digits. Passing the partkey expression (typically
 * `Attr.rowId()`) keeps the value deterministic per part row.
 */
export function tpchRetailPrice(partkey: Expression): Expression {
  const term1 = Expr.mod(Expr.div(partkey, Expr.lit(10)), Expr.lit(20001));
  const term2 = Expr.mul(Expr.lit(100), Expr.mod(partkey, Expr.lit(1000)));
  const numerator = Expr.add(Expr.add(Expr.lit(90000), term1), term2);
  return Expr.div(numerator, Expr.lit(100.0));
}

/**
 * TPC-H manufacturer id — uniform pick over [1, 5] per spec §4.2.3. The
 * raw id drives both p_mfgr ("Manufacturer#N") and the N1..N5 prefix of
 * p_brand. Exposed separately so p_brand's second digit can be drawn
 * independently.
 */
export function tpchMfgrId(): Expression {
  return Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(5) });
}

/** "Manufacturer#<n>" formatter — spec §4.2.3 p_mfgr. */
export function tpchMfgr(mfgrId: Expression): Expression {
  return std.format(Expr.lit("Manufacturer#%d"), mfgrId);
}

/**
 * "Brand#<mn>" formatter — spec §4.2.3 p_brand. m is the manufacturer id
 * (1..5), n is a uniform draw over [1, 5] per the spec. Pass `mfgrId`
 * explicitly so callers can share a single manufacturer id with p_mfgr.
 */
export function tpchBrand(mfgrId: Expression): Expression {
  const sub = Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(5) });
  return std.format(Expr.lit("Brand#%d%d"), mfgrId, sub);
}

/**
 * Clerk string — spec §4.2.3 o_clerk: "Clerk#<7-digit-id>", id drawn
 * uniformly from [1, SF * 1000]. The SF-dependent upper bound keeps
 * clerk population density constant across scales.
 */
export function tpchClerk(maxClerkId: number): Expression {
  const id = Draw.intUniform({ min: Expr.lit(1), max: Expr.lit(maxClerkId) });
  return std.format(Expr.lit("Clerk#%07d"), id);
}

/**
 * TPC-H sparse orderkey formula (spec §4.2.3 / dbgen bm_utils.c).
 * `rowIndex` is the 0-based order row index in [0, rowcount). The
 * mapping keeps 8 consecutive keys, then skips 24 — e.g. rowIndex 0..7
 * yields keys 1..8, rowIndex 8..15 yields keys 33..40, and so on.
 * Max orderkey is 6_000_000 × SF.
 *
 * Formula: ((rowIndex / 8) * 32) + (rowIndex % 8) + 1.
 *
 * Shared between the orders population and the lineitem LookupPop
 * so both derive identical orderkeys from the same entity index.
 */
export function tpchOrderkey(rowIndex: Expression): Expression {
  const hi = Expr.mul(Expr.div(rowIndex, Expr.lit(8)), Expr.lit(32));
  const lo = Expr.mod(rowIndex, Expr.lit(8));
  return Expr.add(Expr.add(hi, lo), Expr.lit(1));
}

/**
 * Stdlib name-bridge helpers. The TS wrapper's `std.*` shortcuts emit
 * snake-case stdlib names; the Go registry keys them in camelCase.
 * Until the wrapper stabilizes we call the Go-side names directly via
 * `std.call`, keeping the TS call sites readable and the intent
 * spec-traceable.
 */
export function tpchDateToDays(date: Expression): Expression {
  return std.call("std.dateToDays", date);
}
export function tpchDaysToDate(days: Expression): Expression {
  return std.call("std.daysToDate", days);
}
export function tpchHashMod(n: Expression, k: Expression): Expression {
  return std.call("std.hashMod", n, k);
}

/**
 * Deterministic orderdate: spec §4.2.3 puts o_orderdate in
 * [STARTDATE, STARTDATE + 2557] (1992-01-01 .. 1998-12-31). We key the
 * offset by a splitmix64-derived hash of the row id so:
 *  - orders and the lineitem `orders` LookupPop produce identical
 *    dates from the same row id (no Draw.* means no attr-path
 *    dependence on the PRNG stream);
 *  - the distribution still covers every day in the window uniformly
 *    at scale.
 */
const TPCH_ORDERDATE_EPOCH_DAYS = 8036; // 1992-01-01 UTC
const TPCH_ORDERDATE_SPAN_DAYS = 2557;  // 1992-01-01 .. 1998-12-31

export function tpchOrderdateExpr(rowIndex: Expression): Expression {
  const offset = tpchHashMod(rowIndex, Expr.lit(TPCH_ORDERDATE_SPAN_DAYS));
  const days = Expr.add(Expr.lit(TPCH_ORDERDATE_EPOCH_DAYS), offset);
  return tpchDaysToDate(days);
}

/**
 * Shape of one distribution inside `distributions.json`. The generator in
 * `cmd/tpch-dists` emits every dict in this form; tx.ts coerces to
 * `Dict.values(...)` at build time.
 */
export interface TpchDistJsonShape {
  columns?: readonly string[];
  weight_sets?: readonly string[];
  rows: ReadonlyArray<{
    values: readonly (string | number)[];
    weights?: readonly number[];
  }>;
}

/** A typed view of the workload-local distributions.json payload. */
export interface TpchDistributions {
  version: string;
  source: string;
  distributions: Record<string, TpchDistJsonShape>;
}
