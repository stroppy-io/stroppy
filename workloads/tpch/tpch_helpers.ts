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
  Alphabet,
  Draw,
  Expr,
  std,
  type Expression,
} from "./datagen.ts";

/**
 * TPC-H "v-string" text helper (spec §4.2.2.14). Rather than encode the
 * full sentence-grammar walk (a moderately complex recursive composition
 * over 9 sub-dicts), we approximate with a pure random-ASCII string over
 * the `enSpc` alphabet for a length uniformly drawn in [min, max]. The
 * statistical shape that matters for query results is the LENGTH
 * distribution and the occurrence of query-predicate literals (e.g.
 * Q13's "special", "requests"); neither relies on the exact grammar.
 *
 * Why this is a legitimate simplification:
 * - q9 `p_name LIKE '%green%'`: p_name is built from the colors vocab
 *   via `Draw.phrase`, NOT from tpchText — so q9 remains accurate.
 * - q13 `o_comment NOT LIKE '%special%requests%'`: with random ASCII
 *   comments, virtually no orders match the pattern. The query still
 *   executes and returns a result set; cardinalities shift but the
 *   framework proves it runs end-to-end. Documented under the top-level
 *   note in tx.ts.
 *
 * When the plan calls for byte-exact TPC-H parity, swap this for a
 * grammar walk composed from `Expr.choose` + `Draw.phrase` over the
 * grammar / np / vp / etc. dicts in distributions.json.
 */
export function tpchText(minLen: number, maxLen: number): Expression {
  return Draw.ascii({
    min: Expr.lit(minLen),
    max: Expr.lit(maxLen),
    alphabet: Alphabet.enSpc,
  });
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
