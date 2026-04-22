// TPC-C-specific TS composition helpers built on top of stdlib primitives
// (Draw / Expr / Alphabet). Kept out of `pkg/datagen/stdlib/` because the
// semantics are spec-specific: the "ORIGINAL" marker shape, the 3-syllable
// c_last cartesian, and the "last 900 per district" deterministic NULL cut
// are all TPC-C §4.3 rules that do not belong in a generic datagen layer.

import { Alphabet, Draw, Expr } from "./datagen.ts";

// Mirror the alphabet range shape used by Draw.ascii without re-exporting
// the generated proto type. `Alphabet.en[number]` collapses to the same
// `{ min: number; max: number }` pair.
type AsciiRange = typeof Alphabet.en[number];

// Spec §4.3.2.3: C_LAST is a 3-syllable concatenation indexed by the three
// base-10 digits of i ∈ [0, 999]. Ten fixed syllables yield 1000 deterministic
// last names. Emitted eagerly so the dict body is materialized once and
// shared across the workload (and, incidentally, read at tx time for the
// by-name lookup branches of Payment / Order-Status).
export const TPCC_SYLLABLES = [
  "BAR", "OUGHT", "ABLE", "PRI", "PRES",
  "ESE", "ANTI", "CALLY", "ATION", "EING",
] as const;

export const C_LAST_DICT: string[] = Array.from({ length: 1000 }, (_, i) => {
  const d0 = Math.floor(i / 100);
  const d1 = Math.floor(i / 10) % 10;
  const d2 = i % 10;
  return TPCC_SYLLABLES[d0] + TPCC_SYLLABLES[d1] + TPCC_SYLLABLES[d2];
});

// Spec §4.3.3.1: i_data / s_data are 26..50 a-strings; in 10% of rows the
// literal "ORIGINAL" must appear at a random position in the string. We
// compose the marked branch as `asciiRange(prefixLen) + "ORIGINAL" +
// asciiRange(suffixLen)`. To keep the assembled length strictly inside
// [minLen, maxLen] with two independent Draws, we pick per-side ranges
// whose extremes still sum to a valid total: the prefix always contributes
// at least ⌈(minLen - markerLen) / 2⌉ and the suffix likewise, and neither
// side exceeds ⌊(maxLen - markerLen) / 2⌋. The position of "ORIGINAL"
// varies per row within that band — not fully uniform across all positions
// in [0, L-8], but the spec's §4.3.3.1 only requires "a random position",
// and every row still carries the marker.
//
// Each call builds two Draw.ascii exprs + one Expr.concat chain; the outer
// 1:9 weighting (see tpccOriginalOr) matches the spec's 10% rate.
export function tpccOriginalInjected(
  minLen: number,
  maxLen: number,
  alphabet: readonly AsciiRange[] = Alphabet.en,
) {
  if (minLen < 26 || maxLen > 50 || maxLen < minLen) {
    throw new Error(
      `tpccOriginalInjected: minLen=${minLen} maxLen=${maxLen} out of spec range`,
    );
  }
  const MARKER = "ORIGINAL";
  const markerLen = MARKER.length; // 8

  // Split the available body length symmetrically across prefix/suffix.
  // bodyMinLen = minLen - markerLen (18 at defaults); half of that
  // rounded up is each side's minimum. bodyMaxLen = maxLen - markerLen
  // (42 at defaults); half rounded down is each side's max. With
  // min=26 / max=50: each side draws length in [9, 21] → total ∈
  // [26, 50].
  const bodyMinLen = minLen - markerLen;
  const bodyMaxLen = maxLen - markerLen;
  const sideMin    = Math.ceil(bodyMinLen / 2);
  const sideMax    = Math.floor(bodyMaxLen / 2);

  const prefix = Draw.ascii({
    min: Expr.lit(sideMin),
    max: Expr.lit(sideMax),
    alphabet,
  });
  const suffix = Draw.ascii({
    min: Expr.lit(sideMin),
    max: Expr.lit(sideMax),
    alphabet,
  });
  return Expr.concat(Expr.concat(prefix, Expr.lit(MARKER)), suffix);
}

// Spec §4.3.3.1: compose an a-string attribute that has "ORIGINAL" at a
// random position in exactly 10% of rows; the remaining 90% are plain
// a-strings of the same length range. 1:9 Expr.choose reproduces the
// required 10% rate with per-row deterministic seeding.
export function tpccOriginalOr(
  minLen: number,
  maxLen: number,
  alphabet: readonly AsciiRange[] = Alphabet.en,
) {
  return Expr.choose([
    { weight: 1, expr: tpccOriginalInjected(minLen, maxLen, alphabet) },
    {
      weight: 9,
      expr: Draw.ascii({
        min: Expr.lit(minLen),
        max: Expr.lit(maxLen),
        alphabet,
      }),
    },
  ]);
}
