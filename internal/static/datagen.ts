/// <reference lib="es2020.bigint" />
/**
 * datagen.ts — Ergonomic TS wrapper over the generated stroppy.datagen proto
 * types. Workload authors compose `InsertSpec` values through six namespaces:
 * `Rel`, `Attr`, `Expr`, `Draw`, `Dict`, `std`.
 *
 * The wrapper hides the oneof-kind boilerplate, converts int64-typed fields
 * between `number`/`bigint` and the protobuf-ts wire form (string), and
 * deduplicates Dict bodies by content so equal-content dicts collapse to a
 * single entry in `InsertSpec.dicts`.
 */
import {
  AsciiRange as PbAsciiRange,
  Attr as PbAttr,
  BinOp_Op,
  BlockRef as PbBlockRef,
  BlockSlot as PbBlockSlot,
  Call as PbCall,
  Choose as PbChoose,
  ChooseBranch as PbChooseBranch,
  Cohort as PbCohort,
  CohortDraw as PbCohortDraw,
  CohortLive as PbCohortLive,
  Degree as PbDegree,
  DictRow as PbDictRow,
  Dict as PbDict,
  DictAt as PbDictAt,
  DrawAscii as PbDrawAscii,
  DrawBernoulli as PbDrawBernoulli,
  DrawDate as PbDrawDate,
  DrawDecimal as PbDrawDecimal,
  DrawDict as PbDrawDict,
  DrawFloatUniform as PbDrawFloatUniform,
  DrawGrammar as PbDrawGrammar,
  DrawIntUniform as PbDrawIntUniform,
  DrawJoint as PbDrawJoint,
  DrawNURand as PbDrawNURand,
  DrawNormal as PbDrawNormal,
  DrawPhrase as PbDrawPhrase,
  DrawZipf as PbDrawZipf,
  Expr as PbExpr,
  InsertMethod,
  InsertSpec as PbInsertSpec,
  Literal as PbLiteral,
  Lookup as PbLookup,
  LookupPop as PbLookupPop,
  Null as PbNull,
  Parallelism as PbParallelism,
  Population as PbPopulation,
  RelSource as PbRelSource,
  Relationship as PbRelationship,
  RowIndex_Kind,
  SCD2 as PbSCD2,
  Side as PbSide,
  StreamDraw as PbStreamDraw,
  Strategy as PbStrategy,
} from "./stroppy.pb.js";

// -------- int64 helpers --------

/** Integer-valued input accepted in slots that map to int64/uint64 on the wire. */
export type Int64Like = number | bigint;

/** Convert Int64Like to the string form protobuf-ts uses for int64 fields. */
function int64ToString(v: Int64Like): string {
  if (typeof v === "bigint") return v.toString();
  if (!Number.isFinite(v) || !Number.isInteger(v)) {
    throw new Error(`datagen: expected integer for int64, got ${v}`);
  }
  return v.toString();
}

function uint64ToString(v: Int64Like): string {
  if (typeof v === "bigint") {
    if (v < BigInt(0)) throw new Error("datagen: uint64 cannot be negative");
    return v.toString();
  }
  if (!Number.isFinite(v) || !Number.isInteger(v) || v < 0) {
    throw new Error(`datagen: expected non-negative integer for uint64, got ${v}`);
  }
  return v.toString();
}

// -------- FNV-1a 64 over a canonical JSON representation --------

const FNV_OFFSET_64 = BigInt("0xcbf29ce484222325");
const FNV_PRIME_64 = BigInt("0x100000001b3");
const MASK_64 = (BigInt(1) << BigInt(64)) - BigInt(1);

/**
 * Deterministic 64-bit FNV-1a returned as hex. Input is treated as the
 * UTF-16 code-unit sequence of `s` encoded as UTF-8; the hash is stable
 * across JS runtimes for the canonical JSON dict bodies we feed it.
 */
function fnv1a64Hex(s: string): string {
  let hash = FNV_OFFSET_64;
  for (let i = 0; i < s.length; i++) {
    const cu = s.charCodeAt(i);
    // Inline UTF-8 encoding of UTF-16 code units. Surrogate pairs are
    // irrelevant here — dict contents are plain JSON text.
    if (cu < 0x80) {
      hash = mixByte(hash, cu);
    } else if (cu < 0x800) {
      hash = mixByte(hash, 0xc0 | (cu >> 6));
      hash = mixByte(hash, 0x80 | (cu & 0x3f));
    } else {
      hash = mixByte(hash, 0xe0 | (cu >> 12));
      hash = mixByte(hash, 0x80 | ((cu >> 6) & 0x3f));
      hash = mixByte(hash, 0x80 | (cu & 0x3f));
    }
  }
  return hash.toString(16).padStart(16, "0");
}

function mixByte(hash: bigint, byte: number): bigint {
  const next = (hash ^ BigInt(byte)) & MASK_64;
  return (next * FNV_PRIME_64) & MASK_64;
}

/** Canonical JSON: object keys sorted, arrays preserved. */
function canonicalJSON(value: unknown): string {
  if (value === null || typeof value !== "object") {
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return "[" + value.map(canonicalJSON).join(",") + "]";
  }
  const obj = value as Record<string, unknown>;
  const keys = Object.keys(obj).sort();
  return (
    "{" +
    keys
      .map((k) => JSON.stringify(k) + ":" + canonicalJSON(obj[k]))
      .join(",") +
    "}"
  );
}

/** Opaque key derived from dict content; stable across runs. */
function dictKey(d: PbDict): string {
  return "d_" + fnv1a64Hex(canonicalJSON(d));
}

// -------- Namespace: Expr --------

function exprLit(lit: PbLiteral): PbExpr {
  return { kind: { oneofKind: "lit", lit } };
}

function binOp(op: BinOp_Op, a: PbExpr, b?: PbExpr): PbExpr {
  return { kind: { oneofKind: "binOp", binOp: { op, a, b } } };
}

function buildBlockRef(slot: string): PbExpr {
  if (!slot) throw new Error("datagen: blockRef requires a slot name");
  const br: PbBlockRef = { slot };
  return { kind: { oneofKind: "blockRef", blockRef: br } };
}

function buildLookup(
  popName: string,
  attrName: string,
  entityIdx: PbExpr,
): PbExpr {
  if (!popName) throw new Error("datagen: Attr.lookup requires a population name");
  if (!attrName) throw new Error("datagen: Attr.lookup requires an attr name");
  const lk: PbLookup = {
    targetPop: popName,
    attrName,
    entityIndex: entityIdx,
  };
  return { kind: { oneofKind: "lookup", lookup: lk } };
}

/** 1970-01-01, the reference date for `std.dateToDays` semantics. */
const EPOCH_DAYS_ORIGIN_MS = 0;
const MS_PER_DAY = 86400000;

function dateToDays(d: Date): number {
  const t = d.getTime();
  if (!Number.isFinite(t)) throw new Error("datagen: invalid Date");
  return Math.floor((t - EPOCH_DAYS_ORIGIN_MS) / MS_PER_DAY);
}

export const Expr = {
  /** Typed scalar literal. `number` → int64 if integer, double otherwise. */
  lit(x: number | bigint | string | boolean | Date): PbExpr {
    if (typeof x === "bigint") {
      return exprLit({ value: { oneofKind: "int64", int64: x.toString() } });
    }
    if (typeof x === "number") {
      if (Number.isInteger(x)) {
        return exprLit({ value: { oneofKind: "int64", int64: x.toString() } });
      }
      return exprLit({ value: { oneofKind: "double", double: x } });
    }
    if (typeof x === "string") {
      return exprLit({ value: { oneofKind: "string", string: x } });
    }
    if (typeof x === "boolean") {
      return exprLit({ value: { oneofKind: "bool", bool: x } });
    }
    if (x instanceof Date) {
      const days = dateToDays(x);
      return exprLit({ value: { oneofKind: "int64", int64: days.toString() } });
    }
    throw new Error(`datagen: Expr.lit: unsupported type ${typeof x}`);
  },

  /**
   * Typed double literal — always emits the `double` oneof arm, even when
   * `x` is integer-valued. Workloads use this for currency / decimal
   * placeholders where the target column is a floating-point type (e.g.
   * YDB's `Double`), and `Expr.lit(0.0)` would otherwise collapse to
   * int64 because `Number.isInteger(0.0)` is true in JS.
   */
  litFloat(x: number): PbExpr {
    if (typeof x !== "number" || !Number.isFinite(x)) {
      throw new Error(`datagen: Expr.litFloat: expected finite number, got ${x}`);
    }
    return exprLit({ value: { oneofKind: "double", double: x } });
  },

  /**
   * Explicit SQL NULL literal. Evaluates to Go nil in the row scratch,
   * which drivers render as NULL. Use this inside `Expr.if` branches
   * that must yield NULL conditionally (e.g. TPC-C `o_carrier_id` when
   * `o_id ∈ [2101, 3000]`, `ol_delivery_d` for undelivered rows).
   */
  litNull(): PbExpr {
    return exprLit({ value: { oneofKind: "null", null: {} } });
  },

  /** Reference another attribute in the current scope. */
  col(name: string): PbExpr {
    if (!name) throw new Error("datagen: Expr.col requires a name");
    return { kind: { oneofKind: "col", col: { name } } };
  },

  /** Typed ternary; only the selected branch evaluates. */
  if(cond: PbExpr, then: PbExpr, else_: PbExpr): PbExpr {
    return { kind: { oneofKind: "if", if: { cond, then, else: else_ } } };
  },

  add: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.ADD, a, b),
  sub: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.SUB, a, b),
  mul: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.MUL, a, b),
  div: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.DIV, a, b),
  mod: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.MOD, a, b),
  concat: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.CONCAT, a, b),
  eq: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.EQ, a, b),
  ne: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.NE, a, b),
  lt: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.LT, a, b),
  le: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.LE, a, b),
  gt: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.GT, a, b),
  ge: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.GE, a, b),
  and: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.AND, a, b),
  or: (a: PbExpr, b: PbExpr) => binOp(BinOp_Op.OR, a, b),
  not: (a: PbExpr) => binOp(BinOp_Op.NOT, a),

  /**
   * Low-level alias for `Attr.blockRef` — reads a named slot on the enclosing
   * Side, resolved against the current outer-side entity. Prefer the Attr
   * namespace at attr-level composition sites.
   */
  blockRef: (slot: string): PbExpr => buildBlockRef(slot),

  /**
   * Weighted pick among a set of Expr branches. Only the selected branch
   * evaluates. At least one branch is required; all weights must be
   * positive. `stream_id` is left 0 — `compile.AssignStreamIDs` fills it
   * in at compile time.
   */
  choose(branches: ReadonlyArray<{ weight: Int64Like; expr: PbExpr }>): PbExpr {
    if (branches.length === 0) {
      throw new Error("datagen: Expr.choose requires at least one branch");
    }
    const pb: PbChooseBranch[] = branches.map((b) => {
      const w = typeof b.weight === "bigint" ? b.weight : BigInt(b.weight);
      if (w <= BigInt(0)) {
        throw new Error("datagen: Expr.choose branch weights must be > 0");
      }
      if (!b.expr) {
        throw new Error("datagen: Expr.choose branch expr is required");
      }
      return { weight: w.toString(), expr: b.expr };
    });
    const choose: PbChoose = { streamId: 0, branches: pb };
    return { kind: { oneofKind: "choose", choose } };
  },
};

// -------- Namespace: std --------

function call(name: string, args: PbExpr[]): PbExpr {
  const c: PbCall = { func: name, args };
  return { kind: { oneofKind: "call", call: c } };
}

/**
 * Typed wrappers for the Go stdlib primitives registered in
 * `pkg/datagen/stdlib/`. Each wrapper validates arity at the TS signature
 * level; runtime signature checks live in the Go registry.
 */
export const std = {
  /** Raw stdlib call escape hatch. Prefer a typed helper below. */
  call(name: string, ...args: PbExpr[]): PbExpr {
    if (!name) throw new Error("datagen: std.call requires a function name");
    return call(name, args);
  },

  /** Go-style format string with typed arguments. */
  format(fmt: PbExpr, ...args: PbExpr[]): PbExpr {
    return call("std.format", [fmt, ...args]);
  },

  /** splitmix64(n) mod k — evenly distributes n across [0, k). */
  hashMod(n: PbExpr, k: PbExpr): PbExpr {
    return call("std.hashMod", [n, k]);
  },

  /** Deterministic UUID v4 derived from a 64-bit seed. */
  uuidSeeded(seed: PbExpr): PbExpr {
    return call("std.uuidSeeded", [seed]);
  },

  /** Convert epoch-days into a date scalar (YYYY-MM-DD on SQL side). */
  daysToDate(days: PbExpr): PbExpr {
    return call("std.daysToDate", [days]);
  },

  /** Convert a date scalar into epoch-days. */
  dateToDays(t: PbExpr): PbExpr {
    return call("std.dateToDays", [t]);
  },

  /** ASCII lowercase. */
  lower(s: PbExpr): PbExpr {
    return call("std.lower", [s]);
  },

  /** ASCII uppercase. */
  upper(s: PbExpr): PbExpr {
    return call("std.upper", [s]);
  },

  /** UTF-8-safe substring. */
  substr(s: PbExpr, i: PbExpr, n: PbExpr): PbExpr {
    return call("std.substr", [s, i, n]);
  },

  /** String rune count. */
  len(s: PbExpr): PbExpr {
    return call("std.len", [s]);
  },

  /** Format any scalar as a string. */
  toString(x: PbExpr): PbExpr {
    return call("std.toString", [x]);
  },

  /**
   * Parse a base-10 integer out of a string scalar. Bridges numeric
   * columns held in string-typed dict rows (dstparse emits all
   * `DictRow.values` as strings on the wire).
   */
  parseInt(x: PbExpr): PbExpr {
    return call("std.parseInt", [x]);
  },

  /**
   * Parse a 64-bit float out of a string scalar. Bridges numeric columns
   * held in string-typed dict rows (dstparse emits all `DictRow.values`
   * as strings on the wire).
   */
  parseFloat(x: PbExpr): PbExpr {
    return call("std.parseFloat", [x]);
  },

  /**
   * Deterministic bijection of [0, n) keyed by `seed`. Iterating `idx`
   * across [0, n) yields each integer in the range exactly once in a
   * shuffled order; same (seed, idx, n) always returns the same output;
   * different seeds produce uncorrelated permutations. Implemented as
   * a cycle-walking 4-round Feistel cipher over a SplitMix64 round
   * function — no per-call state, parallel-safe.
   *
   * Spec reference: TPC-C §4.3.3.1 requires the set of `o_c_id` values
   * in each district to be a permutation of [1, 3000]; per-district
   * `permuteIndex(districtSeed, rowIndex, 3000) + 1` satisfies the
   * requirement without materializing the schedule.
   */
  permuteIndex(seed: PbExpr, idx: PbExpr, n: PbExpr): PbExpr {
    return call("std.permuteIndex", [seed, idx, n]);
  },
};

// -------- Namespace: Dict --------

/**
 * Scalar inline dict, uniform weights. Each entry becomes a one-value row.
 */
function dictValues(values: readonly (string | number | bigint)[]): PbDict {
  const rows: PbDictRow[] = values.map((v) => ({
    values: [toDictString(v)],
    weights: [],
  }));
  return { columns: [], weightSets: [], rows };
}

/**
 * Scalar inline dict with a single default (empty-name) weight set. `values`
 * and `weights` must be parallel and same length.
 */
function dictWeighted(
  values: readonly (string | number | bigint)[],
  weights: readonly Int64Like[],
): PbDict {
  if (values.length !== weights.length) {
    throw new Error(
      `datagen: Dict.weighted: values (${values.length}) and weights (${weights.length}) must be parallel`,
    );
  }
  const rows: PbDictRow[] = values.map((v, i) => ({
    values: [toDictString(v)],
    weights: [int64ToString(weights[i])],
  }));
  return { columns: [], weightSets: [""], rows };
}

function toDictString(v: string | number | bigint): string {
  if (typeof v === "string") return v;
  return v.toString();
}

/**
 * Scalar inline dict carrying several named weight profiles. Callers pick a
 * profile at draw time via `{ weightSet: "<name>" }`. All weight arrays must
 * have the same length as `values`.
 */
function dictMultiWeighted(
  values: readonly string[],
  weights: Readonly<Record<string, readonly number[]>>,
): PbDict {
  const names = Object.keys(weights);
  if (names.length === 0) {
    throw new Error("datagen: Dict.multiWeighted requires at least one weight profile");
  }
  for (const name of names) {
    const arr = weights[name];
    if (arr.length !== values.length) {
      throw new Error(
        `datagen: Dict.multiWeighted: weight profile "${name}" has ` +
          `${arr.length} entries, expected ${values.length}`,
      );
    }
  }
  const rows: PbDictRow[] = values.map((v, i) => {
    const rowWeights = names.map((n) => int64ToString(weights[n][i]));
    return { values: [v], weights: rowWeights };
  });
  return { columns: [], weightSets: names, rows };
}

/**
 * Multi-column inline dict. Each row's `values` length must equal
 * `columns.length`. When no row carries `weights`, the dict is uniform;
 * when any row carries weights, every row must carry the same count, and
 * an unnamed default weight-set is synthesized.
 */
function dictJoint(
  columns: readonly string[],
  rows: ReadonlyArray<{ values: readonly string[]; weights?: readonly Int64Like[] }>,
): PbDict {
  if (columns.length === 0) {
    throw new Error("datagen: Dict.joint requires at least one column");
  }
  const anyWeighted = rows.some((r) => r.weights && r.weights.length > 0);
  const pbRows: PbDictRow[] = rows.map((r, i) => {
    if (r.values.length !== columns.length) {
      throw new Error(
        `datagen: Dict.joint row ${i} has ${r.values.length} values, ` +
          `expected ${columns.length}`,
      );
    }
    const rowWeights = anyWeighted
      ? (r.weights && r.weights.length > 0
          ? (r.weights as readonly Int64Like[]).map((w) => int64ToString(w))
          : [int64ToString(0)])
      : [];
    return { values: Array.from(r.values), weights: rowWeights };
  });
  return {
    columns: Array.from(columns),
    weightSets: anyWeighted ? [""] : [],
    rows: pbRows,
  };
}

/**
 * Multi-column inline dict with N named weight profiles. Each row must carry
 * a `weights` array parallel to `weightSetNames`.
 */
function dictJointWeighted(
  columns: readonly string[],
  weightSetNames: readonly string[],
  rows: ReadonlyArray<{ values: readonly string[]; weights: readonly Int64Like[] }>,
): PbDict {
  if (columns.length === 0) {
    throw new Error("datagen: Dict.jointWeighted requires at least one column");
  }
  if (weightSetNames.length === 0) {
    throw new Error("datagen: Dict.jointWeighted requires at least one weight profile");
  }
  const pbRows: PbDictRow[] = rows.map((r, i) => {
    if (r.values.length !== columns.length) {
      throw new Error(
        `datagen: Dict.jointWeighted row ${i} has ${r.values.length} values, ` +
          `expected ${columns.length}`,
      );
    }
    if (r.weights.length !== weightSetNames.length) {
      throw new Error(
        `datagen: Dict.jointWeighted row ${i} has ${r.weights.length} weights, ` +
          `expected ${weightSetNames.length}`,
      );
    }
    return {
      values: Array.from(r.values),
      weights: (r.weights as readonly Int64Like[]).map((w) => int64ToString(w)),
    };
  });
  return {
    columns: Array.from(columns),
    weightSets: Array.from(weightSetNames),
    rows: pbRows,
  };
}

/**
 * Shape accepted by `Dict.fromJson` — the canonical output of
 * `cmd/dstparse`. `columns` and `weight_sets` default to empty, `rows`
 * carries values and optional parallel weights.
 */
export interface DictJsonShape {
  columns?: readonly string[];
  weight_sets?: readonly string[];
  rows: ReadonlyArray<{
    values: readonly (string | number | bigint)[];
    weights?: readonly Int64Like[];
  }>;
}

/**
 * Coerce a dstparse-shaped JSON payload into a `PbDict`. Auto-detects
 * scalar vs joint shape: omitted/empty `columns` produce a scalar dict;
 * weight arrays are preserved row-by-row.
 */
function dictFromJson(json: DictJsonShape): PbDict {
  if (!json || !Array.isArray(json.rows)) {
    throw new Error("datagen: Dict.fromJson: missing rows[]");
  }
  const columns = json.columns ? [...json.columns] : [];
  const weightSets = json.weight_sets ? [...json.weight_sets] : [];
  const rows: PbDictRow[] = json.rows.map((r, i) => {
    if (!Array.isArray(r.values)) {
      throw new Error(`datagen: Dict.fromJson row ${i} missing values[]`);
    }
    if (weightSets.length > 0) {
      const weights = r.weights ?? [];
      if (weights.length !== weightSets.length) {
        throw new Error(
          `datagen: Dict.fromJson row ${i} has ${weights.length} weights, ` +
            `expected ${weightSets.length}`,
        );
      }
      return {
        values: r.values.map(toDictString),
        weights: (weights as readonly Int64Like[]).map((w) => int64ToString(w)),
      };
    }
    return {
      values: r.values.map(toDictString),
      weights: r.weights
        ? (r.weights as readonly Int64Like[]).map((w) => int64ToString(w))
        : [],
    };
  });
  return { columns, weightSets, rows };
}

export const Dict = {
  values: dictValues,
  weighted: dictWeighted,
  multiWeighted: dictMultiWeighted,
  joint: dictJoint,
  jointWeighted: dictJointWeighted,
  fromJson: dictFromJson,
};

/** Anything accepted where a Dict reference is expected. */
export type DictRef = PbDict | string;

/** Anything accepted where a vocabulary Dict is expected — same as DictRef. */
export type DictLike = DictRef;

// -------- Namespace: Attr --------

export const Attr = {
  /** 0-based row counter. `kind` defaults to UNSPECIFIED (treated as ENTITY). */
  rowIndex(kind: RowIndex_Kind = RowIndex_Kind.UNSPECIFIED): PbExpr {
    return { kind: { oneofKind: "rowIndex", rowIndex: { kind } } };
  },

  /** 1-based convenience = rowIndex() + 1. */
  rowId(): PbExpr {
    return Expr.add(
      Attr.rowIndex(RowIndex_Kind.UNSPECIFIED),
      Expr.lit(BigInt(1)),
    );
  },

  /**
   * Dict row read. `dict` is either a Dict built by `Dict.*` (registered with
   * the owning `Rel.table` call) or an already-assigned opaque key string.
   */
  dictAt(dict: DictRef, index: PbExpr, column?: string): PbExpr {
    const dictKeyStr =
      typeof dict === "string" ? dict : registerInlineDict(dict);
    const da: PbDictAt = {
      dictKey: dictKeyStr,
      index,
      column: column ?? "",
    };
    return { kind: { oneofKind: "dictAt", dictAt: da } };
  },

  /**
   * Dict row read coerced to int64 via `std.parseInt`. Shortcut for
   * numeric dict columns that arrive as strings on the wire (dstparse
   * emits all `DictRow.values` as strings).
   */
  dictAtInt(dict: DictRef, index: PbExpr, column?: string): PbExpr {
    return std.parseInt(Attr.dictAt(dict, index, column));
  },

  /**
   * Dict row read coerced to float64 via `std.parseFloat`. Shortcut for
   * numeric dict columns that arrive as strings on the wire (dstparse
   * emits all `DictRow.values` as strings).
   */
  dictAtFloat(dict: DictRef, index: PbExpr, column?: string): PbExpr {
    return std.parseFloat(Attr.dictAt(dict, index, column));
  },

  /**
   * Cross-population attribute read. `popName` names the iter-side population
   * or an entry in the enclosing `RelSource.lookup_pops`; `entityIdx`
   * evaluates to the target row index.
   */
  lookup(popName: string, attrName: string, entityIdx: PbExpr): PbExpr {
    return buildLookup(popName, attrName, entityIdx);
  },

  /**
   * Read a named block slot on the enclosing Side, resolved against the
   * current outer-side entity. Mirrored by `Expr.blockRef` for low-level use.
   */
  blockRef(slot: string): PbExpr {
    return buildBlockRef(slot);
  },

  /**
   * Draw one entity ID from the named cohort's schedule at position `slot`.
   * `bucketKey` overrides the Cohort's default bucket-key expression; omit
   * to inherit the default.
   */
  cohortDraw(name: string, slot: PbExpr, bucketKey?: PbExpr): PbExpr {
    if (!name) throw new Error("datagen: Attr.cohortDraw requires a cohort name");
    if (!slot) throw new Error("datagen: Attr.cohortDraw requires a slot expr");
    const cd: PbCohortDraw = { name, slot, bucketKey };
    return { kind: { oneofKind: "cohortDraw", cohortDraw: cd } };
  },

  /**
   * Report whether the named cohort's bucket is active for the given key
   * (or its default bucket-key when unset). Returns an int64 1/0 at the
   * runtime layer.
   */
  cohortLive(name: string, bucketKey?: PbExpr): PbExpr {
    if (!name) throw new Error("datagen: Attr.cohortLive requires a cohort name");
    const cl: PbCohortLive = { name, bucketKey };
    return { kind: { oneofKind: "cohortLive", cohortLive: cl } };
  },
};


// -------- Dict registry --------

/**
 * Inline-dict accumulator. `Attr.dictAt(Dict.values([...]), ...)` stores the
 * dict body here keyed by its content hash; `Rel.table` drains the map and
 * emits each unique dict exactly once in `InsertSpec.dicts`. The map is
 * module-global but dedup-by-key is safe across concurrent table builds —
 * equal content maps to equal keys.
 */
const pendingDicts = new Map<string, PbDict>();

function registerInlineDict(d: PbDict): string {
  const key = dictKey(d);
  if (!pendingDicts.has(key)) pendingDicts.set(key, d);
  return key;
}

// -------- Namespace: Deg / Strat --------

/** Degree builders for Relationship Sides. */
export const Deg = {
  /** Constant inner-row count per outer entity. */
  fixed(count: Int64Like): PbDegree {
    return {
      kind: {
        oneofKind: "fixed",
        fixed: { count: int64ToString(count) },
      },
    };
  },

  /** Uniform-draw inner-row count per outer entity. Inclusive bounds. */
  uniform(min: Int64Like, max: Int64Like): PbDegree {
    return {
      kind: {
        oneofKind: "uniform",
        uniform: { min: int64ToString(min), max: int64ToString(max) },
      },
    };
  },
};

/** Strategy builders for pairing outer entities to inner ones on a Side. */
export const Strat = {
  /** Sequential walk over inner entities. */
  sequential(): PbStrategy {
    return { kind: { oneofKind: "sequential", sequential: {} } };
  },
  /** Hash-of-outer-index pairing. */
  hash(): PbStrategy {
    return { kind: { oneofKind: "hash", hash: {} } };
  },
  /** Equitable allocation, spreading inner entities evenly across outer. */
  equitable(): PbStrategy {
    return { kind: { oneofKind: "equitable", equitable: {} } };
  },
};

// -------- Namespace: Rel --------

/** Options accepted by `Rel.side`. */
export interface RelSideOpts {
  /** Inner-row count per outer entity. Build via `Deg.fixed` / `Deg.uniform`. */
  degree: PbDegree;
  /** Outer→inner pairing strategy. Build via `Strat.*`. */
  strategy: PbStrategy;
  /** Optional block slots: slot name → expr evaluated once per outer entity. */
  blockSlots?: Record<string, PbExpr>;
}

/** Options accepted by `Rel.lookupPop`. */
export interface RelLookupPopOpts {
  /** Population identifier; referenced by `Attr.lookup(popName, …)`. */
  name: string;
  /** Entity count for the lookup population. */
  size: Int64Like;
  /** Column → generating expression (or expr + null spec). */
  attrs: Record<string, PbExpr | { expr: PbExpr; null?: NullSpec }>;
  /** Explicit column order; must cover exactly the keys of `attrs`. */
  columnOrder?: readonly string[];
  /** Root PRNG seed; currently unused at the LookupPop proto level. */
  seed?: Int64Like;
  /**
   * Whether this population is pure (read through Lookup only, never
   * iterated). Defaults to true — the common case for lookup pops.
   */
  pure?: boolean;
}

/** Options accepted by `Rel.table`. */
export interface RelTableOpts {
  /** Entity count for the population. */
  size: Int64Like;
  /** Root PRNG seed; 0 picks a random seed per run. */
  seed?: Int64Like;
  /** Column name → generating expression. Insertion order drives `columnOrder`. */
  attrs: Record<string, PbExpr>;
  /** Explicit column order override; must cover exactly the keys of `attrs`. */
  columnOrder?: readonly string[];
  /** Wire protocol for row insertion. */
  method?: InsertMethod;
  /** Worker hint; clamped by the Loader. */
  parallelism?: number;
  /**
   * Pre-registered dict bodies keyed by their opaque string. Inline dicts
   * declared within attrs are merged automatically.
   */
  dicts?: Record<string, PbDict>;
  /** Relationships this table participates in; see `Rel.relationship`. */
  relationships?: PbRelationship[];
  /** Name of the relationship driving iteration for this table. */
  iter?: string;
  /** Pure sibling populations readable via `Attr.lookup`. */
  lookupPops?: PbLookupPop[];
  /** Named cohort schedules readable via `Attr.cohortDraw` / `Attr.cohortLive`. */
  cohorts?: PbCohort[];
  /**
   * SCD-2 row-split descriptor. When set, the runtime auto-injects
   * values for `startCol` and `endCol` based on a boundary row index;
   * both columns must appear in `columnOrder` but not in `attrs`.
   */
  scd2?: PbSCD2;
}

/**
 * Build an `InsertSpec`-shaped plain object for a single-table load. Inline
 * dicts referenced from attrs are deduplicated and emitted once under
 * `InsertSpec.dicts`.
 */
function relTable(name: string, opts: RelTableOpts): PbInsertSpec {
  if (!name) throw new Error("datagen: Rel.table requires a table name");

  const pbAttrs: PbAttr[] = Object.entries(opts.attrs).map(
    ([attrName, expr]) => ({ name: attrName, expr }),
  );

  const attrKeys = Object.keys(opts.attrs);
  // SCD-2-managed columns live in columnOrder but not in attrs; pass
  // their names to validateColumnOrder so they survive the unknown-attr
  // check. Default columnOrder is attrKeys + scd2 pair appended in the
  // order the spec declares them.
  const scd2Names: string[] = opts.scd2
    ? [opts.scd2.startCol, opts.scd2.endCol]
    : [];
  const defaultColumnOrder = [...attrKeys, ...scd2Names];
  const columnOrder = opts.columnOrder
    ? [...opts.columnOrder]
    : defaultColumnOrder;
  validateColumnOrder(columnOrder, attrKeys, scd2Names);

  const population: PbPopulation = {
    name,
    size: int64ToString(opts.size),
    pure: false,
  };

  const source: PbRelSource = {
    population,
    attrs: pbAttrs,
    columnOrder,
    relationships: opts.relationships ? [...opts.relationships] : [],
    iter: opts.iter ?? "",
    cohorts: opts.cohorts ? [...opts.cohorts] : [],
    lookupPops: opts.lookupPops ? [...opts.lookupPops] : [],
    scd2: opts.scd2,
  };

  const parallelism: PbParallelism = {
    workers: opts.parallelism ?? 0,
  };

  // Dict emission: dicts referenced from this table's attrs, from any
  // lookup-pop attrs, and from block-slot expressions on relationship sides.
  const referenced = collectDictKeys(pbAttrs);
  for (const lp of source.lookupPops) {
    for (const a of lp.attrs) {
      if (a.expr) walkExpr(a.expr, referenced);
    }
  }
  for (const rel of source.relationships) {
    for (const side of rel.sides) {
      for (const slot of side.blockSlots) {
        if (slot.expr) walkExpr(slot.expr, referenced);
      }
    }
  }
  for (const c of source.cohorts) {
    if (c.bucketKey) walkExpr(c.bucketKey, referenced);
  }
  if (source.scd2) {
    if (source.scd2.boundary) walkExpr(source.scd2.boundary, referenced);
    if (source.scd2.historicalStart) walkExpr(source.scd2.historicalStart, referenced);
    if (source.scd2.historicalEnd) walkExpr(source.scd2.historicalEnd, referenced);
    if (source.scd2.currentStart) walkExpr(source.scd2.currentStart, referenced);
    if (source.scd2.currentEnd) walkExpr(source.scd2.currentEnd, referenced);
  }
  const dicts: { [key: string]: PbDict } = {};
  if (opts.dicts) {
    for (const [k, v] of Object.entries(opts.dicts)) {
      if (referenced.has(k)) dicts[k] = v;
    }
  }
  for (const key of referenced) {
    if (dicts[key]) continue;
    const body = pendingDicts.get(key);
    if (!body) {
      throw new Error(
        `datagen: dict "${key}" referenced but not registered; ` +
          "pass it via opts.dicts or build it with Dict.*",
      );
    }
    dicts[key] = body;
  }
  // Pending dicts stay resident for other tables; GC happens on the next
  // pass that references them. Harmless because dict keys are content-hashed.

  return {
    table: name,
    seed: uint64ToString(opts.seed ?? 0),
    method: opts.method ?? InsertMethod.PLAIN_QUERY,
    parallelism,
    source,
    dicts,
  };
}

/** Recursive walk collecting every `dictKey` referenced under an attr list. */
function collectDictKeys(attrs: readonly PbAttr[]): Set<string> {
  const out = new Set<string>();
  for (const a of attrs) {
    if (a.expr) walkExpr(a.expr, out);
  }
  return out;
}

function walkExpr(e: PbExpr, out: Set<string>): void {
  const k = e.kind;
  switch (k.oneofKind) {
    case "dictAt":
      out.add(k.dictAt.dictKey);
      if (k.dictAt.index) walkExpr(k.dictAt.index, out);
      return;
    case "binOp":
      if (k.binOp.a) walkExpr(k.binOp.a, out);
      if (k.binOp.b) walkExpr(k.binOp.b, out);
      return;
    case "call":
      for (const arg of k.call.args) walkExpr(arg, out);
      return;
    case "if":
      if (k.if.cond) walkExpr(k.if.cond, out);
      if (k.if.then) walkExpr(k.if.then, out);
      if (k.if.else) walkExpr(k.if.else, out);
      return;
    case "lookup":
      if (k.lookup.entityIndex) walkExpr(k.lookup.entityIndex, out);
      return;
    case "streamDraw":
      walkStreamDraw(k.streamDraw, out);
      return;
    case "choose":
      for (const br of k.choose.branches) {
        if (br.expr) walkExpr(br.expr, out);
      }
      return;
    case "cohortDraw":
      if (k.cohortDraw.slot) walkExpr(k.cohortDraw.slot, out);
      if (k.cohortDraw.bucketKey) walkExpr(k.cohortDraw.bucketKey, out);
      return;
    case "cohortLive":
      if (k.cohortLive.bucketKey) walkExpr(k.cohortLive.bucketKey, out);
      return;
    case "blockRef":
    case "col":
    case "rowIndex":
    case "lit":
    case undefined:
      return;
    default:
      return;
  }
}

function walkStreamDraw(sd: PbStreamDraw, out: Set<string>): void {
  const arm = sd.draw;
  switch (arm.oneofKind) {
    case "intUniform":
      if (arm.intUniform.min) walkExpr(arm.intUniform.min, out);
      if (arm.intUniform.max) walkExpr(arm.intUniform.max, out);
      return;
    case "floatUniform":
      if (arm.floatUniform.min) walkExpr(arm.floatUniform.min, out);
      if (arm.floatUniform.max) walkExpr(arm.floatUniform.max, out);
      return;
    case "normal":
      if (arm.normal.min) walkExpr(arm.normal.min, out);
      if (arm.normal.max) walkExpr(arm.normal.max, out);
      return;
    case "zipf":
      if (arm.zipf.min) walkExpr(arm.zipf.min, out);
      if (arm.zipf.max) walkExpr(arm.zipf.max, out);
      return;
    case "decimal":
      if (arm.decimal.min) walkExpr(arm.decimal.min, out);
      if (arm.decimal.max) walkExpr(arm.decimal.max, out);
      return;
    case "ascii":
      if (arm.ascii.minLen) walkExpr(arm.ascii.minLen, out);
      if (arm.ascii.maxLen) walkExpr(arm.ascii.maxLen, out);
      return;
    case "dict":
      out.add(arm.dict.dictKey);
      return;
    case "joint":
      out.add(arm.joint.dictKey);
      return;
    case "phrase":
      out.add(arm.phrase.vocabKey);
      if (arm.phrase.minWords) walkExpr(arm.phrase.minWords, out);
      if (arm.phrase.maxWords) walkExpr(arm.phrase.maxWords, out);
      return;
    case "grammar":
      out.add(arm.grammar.rootDict);
      for (const k of Object.values(arm.grammar.phrases ?? {})) out.add(k);
      for (const k of Object.values(arm.grammar.leaves ?? {})) out.add(k);
      if (arm.grammar.maxLen) walkExpr(arm.grammar.maxLen, out);
      if (arm.grammar.minLen) walkExpr(arm.grammar.minLen, out);
      return;
    case "nurand":
    case "bernoulli":
    case "date":
    case undefined:
      return;
    default:
      return;
  }
}

function validateColumnOrder(
  order: readonly string[],
  keys: readonly string[],
  scd2Names: readonly string[] = [],
): void {
  const expectedLen = keys.length + scd2Names.length;
  if (order.length !== expectedLen) {
    throw new Error(
      `datagen: columnOrder length ${order.length} must equal attrs+scd2 count ${expectedLen}`,
    );
  }
  const keySet = new Set(keys);
  const scd2Set = new Set(scd2Names);
  for (const s of scd2Names) {
    if (keySet.has(s)) {
      throw new Error(
        `datagen: scd2 column "${s}" must not also be declared in attrs`,
      );
    }
  }
  const seen = new Set<string>();
  for (const name of order) {
    const isAttr = keySet.has(name);
    const isScd2 = scd2Set.has(name);
    if (!isAttr && !isScd2) {
      throw new Error(`datagen: columnOrder references unknown attr "${name}"`);
    }
    if (seen.has(name)) {
      throw new Error(`datagen: columnOrder duplicates attr "${name}"`);
    }
    seen.add(name);
  }
}

/** Build a Relationship wrapping two or more Sides under a stable name. */
function relRelationship(name: string, sides: PbSide[]): PbRelationship {
  if (!name) throw new Error("datagen: Rel.relationship requires a name");
  if (sides.length < 2) {
    throw new Error(
      `datagen: Rel.relationship "${name}" needs at least two sides`,
    );
  }
  return { name, sides: [...sides] };
}

/** Build a Side projecting one population into a Relationship. */
function relSide(population: string, opts: RelSideOpts): PbSide {
  if (!population) throw new Error("datagen: Rel.side requires a population");
  const blockSlots: PbBlockSlot[] = opts.blockSlots
    ? Object.entries(opts.blockSlots).map(([name, expr]) => ({ name, expr }))
    : [];
  return {
    population,
    degree: opts.degree,
    strategy: opts.strategy,
    blockSlots,
  };
}

/** Options accepted by `Rel.scd2`. */
export interface RelSCD2Opts {
  /** Column name receiving the start-of-validity value. */
  startCol: string;
  /** Column name receiving the end-of-validity value. */
  endCol: string;
  /** Row-index boundary; rows with index < boundary get the historical pair. */
  boundary: PbExpr;
  /** Start-of-validity value for the historical slice. */
  historicalStart: PbExpr;
  /** End-of-validity value for the historical slice. */
  historicalEnd: PbExpr;
  /** Start-of-validity value for the current slice. */
  currentStart: PbExpr;
  /** End-of-validity value for the current slice; omit for SQL NULL. */
  currentEnd?: PbExpr;
}

/** Build an SCD-2 row-split descriptor for `Rel.table({ scd2 })`. */
function relSCD2(opts: RelSCD2Opts): PbSCD2 {
  if (!opts.startCol) throw new Error("datagen: Rel.scd2 requires startCol");
  if (!opts.endCol) throw new Error("datagen: Rel.scd2 requires endCol");
  if (opts.startCol === opts.endCol) {
    throw new Error("datagen: Rel.scd2 startCol and endCol must differ");
  }
  return {
    startCol: opts.startCol,
    endCol: opts.endCol,
    boundary: opts.boundary,
    historicalStart: opts.historicalStart,
    historicalEnd: opts.historicalEnd,
    currentStart: opts.currentStart,
    currentEnd: opts.currentEnd,
  };
}

/** Build a LookupPop — a pure sibling population readable via `Attr.lookup`. */
function relLookupPop(opts: RelLookupPopOpts): PbLookupPop {
  if (!opts.name) throw new Error("datagen: Rel.lookupPop requires a name");
  const pbAttrs: PbAttr[] = Object.entries(opts.attrs).map(
    ([attrName, v]) => {
      if ("expr" in v && v.expr) {
        return { name: attrName, expr: v.expr, null: v.null };
      }
      return { name: attrName, expr: v as PbExpr };
    },
  );
  const attrKeys = Object.keys(opts.attrs);
  const columnOrder = opts.columnOrder ? [...opts.columnOrder] : attrKeys;
  validateColumnOrder(columnOrder, attrKeys);
  const population: PbPopulation = {
    name: opts.name,
    size: int64ToString(opts.size),
    pure: opts.pure ?? true,
  };
  return { population, attrs: pbAttrs, columnOrder };
}

/** Options accepted by `Rel.cohort`. */
export interface RelCohortOpts {
  /** Stable identifier referenced by Attr.cohortDraw / Attr.cohortLive. */
  name: string;
  /** Number of entities drawn per active bucket. */
  cohortSize: Int64Like;
  /** Inclusive lower bound on the entity-ID range drawn from. */
  entityMin: Int64Like;
  /** Inclusive upper bound on the entity-ID range drawn from. */
  entityMax: Int64Like;
  /** Default bucket-key expression; per-call overrides are accepted. */
  bucketKey?: PbExpr;
  /** Every N-th bucket is active. 0 or 1 leaves every bucket active. */
  activeEvery?: Int64Like;
  /** Modulus used to collapse bucket keys into the persistent slice. */
  persistenceMod?: Int64Like;
  /** Fraction of cohortSize drawn from the persistent slice. */
  persistenceRatio?: number;
  /** Per-cohort seed salt providing independence from other cohorts. */
  seedSalt?: Int64Like;
}

/** Build a Cohort proto for attachment to `RelTableOpts.cohorts`. */
function relCohort(opts: RelCohortOpts): PbCohort {
  if (!opts.name) throw new Error("datagen: Rel.cohort requires a name");
  return {
    name: opts.name,
    cohortSize: int64ToString(opts.cohortSize),
    entityMin: int64ToString(opts.entityMin),
    entityMax: int64ToString(opts.entityMax),
    bucketKey: opts.bucketKey,
    activeEvery: int64ToString(opts.activeEvery ?? 0),
    persistenceMod: int64ToString(opts.persistenceMod ?? 0),
    persistenceRatio: opts.persistenceRatio ?? 0,
    seedSalt: uint64ToString(opts.seedSalt ?? 0),
  };
}

export const Rel = {
  table: relTable,
  relationship: relRelationship,
  side: relSide,
  lookupPop: relLookupPop,
  scd2: relSCD2,
  cohort: relCohort,
};

// -------- Alphabets (for Draw.ascii) --------

/**
 * ASCII code-point ranges used by `Draw.ascii`. Each entry is a
 * contiguous [min, max] sampled with uniform width. Names mirror the
 * legacy `AB.*` semantics exactly.
 */
export const Alphabet: {
  readonly en: readonly PbAsciiRange[];
  readonly enNum: readonly PbAsciiRange[];
  readonly num: readonly PbAsciiRange[];
  readonly enUpper: readonly PbAsciiRange[];
  readonly enSpc: readonly PbAsciiRange[];
  readonly enNumSpc: readonly PbAsciiRange[];
  readonly ascii: readonly PbAsciiRange[];
} = {
  en: [
    { min: 65, max: 90 },
    { min: 97, max: 122 },
  ],
  enNum: [
    { min: 65, max: 90 },
    { min: 97, max: 122 },
    { min: 48, max: 57 },
  ],
  num: [{ min: 48, max: 57 }],
  enUpper: [{ min: 65, max: 90 }],
  enSpc: [
    { min: 65, max: 90 },
    { min: 97, max: 122 },
    { min: 32, max: 33 },
  ],
  enNumSpc: [
    { min: 65, max: 90 },
    { min: 97, max: 122 },
    { min: 48, max: 57 },
    { min: 32, max: 33 },
  ],
  ascii: [{ min: 32, max: 126 }],
};

// -------- Namespace: Draw --------

/** Wrap one StreamDraw arm into an Expr with stream_id=0 (filled at compile). */
function streamDrawExpr(draw: PbStreamDraw["draw"]): PbExpr {
  const sd: PbStreamDraw = { streamId: 0, draw };
  return { kind: { oneofKind: "streamDraw", streamDraw: sd } };
}

/** Opts shared by draws that carry inclusive `min`/`max` bounds. */
export interface DrawRangeOpts {
  min: PbExpr;
  max: PbExpr;
}

/** Opts accepted by `Draw.normal`. */
export interface DrawNormalOpts extends DrawRangeOpts {
  /** Span divisor — larger values tighten the distribution. Default 3.0. */
  screw?: number;
}

/** Opts accepted by `Draw.zipf`. */
export interface DrawZipfOpts extends DrawRangeOpts {
  /** Power-law exponent; exponents <= 1 are internally nudged. */
  exponent: number;
}

/** Opts accepted by `Draw.nurand`. */
export interface DrawNURandOpts {
  a: Int64Like;
  x: Int64Like;
  y: Int64Like;
  cSalt?: Int64Like;
}

/** Opts accepted by `Draw.bernoulli`. */
export interface DrawBernoulliOpts {
  p: number;
}

/** Opts accepted by `Draw.date`. Bounds are JS Dates, converted to epoch days. */
export interface DrawDateOpts {
  minDate: Date;
  maxDate: Date;
}

/** Opts accepted by `Draw.decimal`. */
export interface DrawDecimalOpts extends DrawRangeOpts {
  /** Fractional digits retained after rounding. */
  scale: number;
}

/** Opts accepted by `Draw.ascii`. */
export interface DrawAsciiOpts {
  min: PbExpr;
  max: PbExpr;
  /** Code-point ranges sampled uniformly by width. Defaults to `Alphabet.en`. */
  alphabet?: readonly PbAsciiRange[];
}

/** Opts accepted by `Draw.phrase`. */
export interface DrawPhraseOpts {
  /** Vocabulary dict — either a dict body or a pre-registered key. */
  vocab: DictLike;
  minWords: PbExpr;
  maxWords: PbExpr;
  /** String joining adjacent words; defaults to a single space. */
  separator?: string;
}

/** Opts accepted by `Draw.dict`. */
export interface DrawDictOpts {
  /** Named weight profile; empty/omitted selects uniform / default. */
  weightSet?: string;
}

/** Opts accepted by `Draw.joint`. */
export interface DrawJointOpts {
  /** Named weight profile; empty/omitted selects uniform / default. */
  weightSet?: string;
  /** Tuple-scope identifier reserved for sharing one draw across columns. */
  tupleScope?: number;
}

/** Opts accepted by `Draw.grammar`. */
export interface DrawGrammarOpts {
  /** Root template dict: sentence templates mixing letters and literals. */
  rootDict: DictLike;
  /**
   * Phrase-level nonterminals: letter → dict whose rows are phrase templates
   * (e.g. `N` → `np` dict with rows `"N"`, `"J N"`, `"J, J N"`). Each picked
   * phrase is tokenized and its letters resolve via `leaves`.
   */
  phrases?: Record<string, DictLike>;
  /**
   * Leaf nonterminals: letter → dict whose rows are individual words (e.g.
   * `N` → `nouns`, `V` → `verbs`). Must cover every letter the root or a
   * phrase may emit; unresolved letters error out at evaluation time.
   */
  leaves: Record<string, DictLike>;
  /** Maximum character length of the final joined string; over-long walks truncate. */
  maxLen: PbExpr | number | bigint;
  /**
   * Minimum character length. When set and a walk produces a shorter string,
   * the evaluator re-walks up to 8 times to satisfy. Omit to accept any
   * length up to `maxLen`.
   */
  minLen?: PbExpr | number | bigint;
}

/** Resolve a DictLike down to a registered opaque key. */
function resolveDictKey(d: DictLike): string {
  return typeof d === "string" ? d : registerInlineDict(d);
}

/**
 * Stream-draw primitives. Every builder emits an `Expr` wrapping a
 * `StreamDraw` oneof; `stream_id` is left 0 — `compile.AssignStreamIDs`
 * populates it at runtime-construction time.
 */
export const Draw = {
  /** Uniform integer on [min, max] inclusive. */
  intUniform(opts: DrawRangeOpts): PbExpr {
    const arm: PbDrawIntUniform = { min: opts.min, max: opts.max };
    return streamDrawExpr({ oneofKind: "intUniform", intUniform: arm });
  },

  /** Uniform float on [min, max). */
  floatUniform(opts: DrawRangeOpts): PbExpr {
    const arm: PbDrawFloatUniform = { min: opts.min, max: opts.max };
    return streamDrawExpr({ oneofKind: "floatUniform", floatUniform: arm });
  },

  /** Truncated normal clamped to [min, max]. `screw` defaults to 3.0. */
  normal(opts: DrawNormalOpts): PbExpr {
    const arm: PbDrawNormal = {
      min: opts.min,
      max: opts.max,
      screw: opts.screw ?? 0,
    };
    return streamDrawExpr({ oneofKind: "normal", normal: arm });
  },

  /** Zipfian power-law over [min, max]. */
  zipf(opts: DrawZipfOpts): PbExpr {
    const arm: PbDrawZipf = {
      min: opts.min,
      max: opts.max,
      exponent: opts.exponent,
    };
    return streamDrawExpr({ oneofKind: "zipf", zipf: arm });
  },

  /** TPC-C §2.1.6 NURand(A, x, y) with optional `cSalt`. */
  nurand(opts: DrawNURandOpts): PbExpr {
    const arm: PbDrawNURand = {
      a: int64ToString(opts.a),
      x: int64ToString(opts.x),
      y: int64ToString(opts.y),
      cSalt: uint64ToString(opts.cSalt ?? 0),
    };
    return streamDrawExpr({ oneofKind: "nurand", nurand: arm });
  },

  /** Bernoulli {0, 1} with probability p of 1. */
  bernoulli(opts: DrawBernoulliOpts): PbExpr {
    const arm: PbDrawBernoulli = { p: opts.p };
    return streamDrawExpr({ oneofKind: "bernoulli", bernoulli: arm });
  },

  /** Uniform date over an inclusive Date range; bounds convert to epoch days. */
  date(opts: DrawDateOpts): PbExpr {
    const arm: PbDrawDate = {
      minDaysEpoch: dateToDays(opts.minDate).toString(),
      maxDaysEpoch: dateToDays(opts.maxDate).toString(),
    };
    return streamDrawExpr({ oneofKind: "date", date: arm });
  },

  /** Uniform decimal rounded to `scale` fractional digits. */
  decimal(opts: DrawDecimalOpts): PbExpr {
    if (!Number.isInteger(opts.scale) || opts.scale < 0) {
      throw new Error(`datagen: Draw.decimal: scale must be >= 0 integer, got ${opts.scale}`);
    }
    const arm: PbDrawDecimal = {
      min: opts.min,
      max: opts.max,
      scale: opts.scale,
    };
    return streamDrawExpr({ oneofKind: "decimal", decimal: arm });
  },

  /** Random ASCII string drawn from `alphabet`; defaults to `Alphabet.en`. */
  ascii(opts: DrawAsciiOpts): PbExpr {
    const alphabet = opts.alphabet ?? Alphabet.en;
    if (alphabet.length === 0) {
      throw new Error("datagen: Draw.ascii requires at least one alphabet range");
    }
    const arm: PbDrawAscii = {
      minLen: opts.min,
      maxLen: opts.max,
      alphabet: alphabet.map((r) => ({ min: r.min, max: r.max })),
    };
    return streamDrawExpr({ oneofKind: "ascii", ascii: arm });
  },

  /** Space-joined word sequence drawn from `vocab`. */
  phrase(opts: DrawPhraseOpts): PbExpr {
    const vocabKey = resolveDictKey(opts.vocab);
    const arm: PbDrawPhrase = {
      vocabKey,
      minWords: opts.minWords,
      maxWords: opts.maxWords,
      separator: opts.separator ?? " ",
    };
    return streamDrawExpr({ oneofKind: "phrase", phrase: arm });
  },

  /** Weighted or uniform pick from a scalar Dict. */
  dict(d: DictLike, opts?: DrawDictOpts): PbExpr {
    const dictKeyStr = resolveDictKey(d);
    const arm: PbDrawDict = {
      dictKey: dictKeyStr,
      weightSet: opts?.weightSet ?? "",
    };
    return streamDrawExpr({ oneofKind: "dict", dict: arm });
  },

  /** Tuple draw from a joint Dict, returning `column`'s value. */
  joint(d: DictLike, column: string, opts?: DrawJointOpts): PbExpr {
    if (!column) throw new Error("datagen: Draw.joint requires a column name");
    const dictKeyStr = resolveDictKey(d);
    const arm: PbDrawJoint = {
      dictKey: dictKeyStr,
      column,
      tupleScope: opts?.tupleScope ?? 0,
      weightSet: opts?.weightSet ?? "",
    };
    return streamDrawExpr({ oneofKind: "joint", joint: arm });
  },

  /**
   * Two-phase template walker (spec §4.2.2.14). Picks a sentence from
   * `rootDict`; for every single-uppercase-ASCII-letter token, either
   * expands the phrase template found in `phrases[letter]` (one level
   * deep, sub-letters resolve via `leaves`) or emits a leaf word from
   * `leaves[letter]`. Result is truncated to `maxLen` characters; when
   * `minLen` is set, the evaluator re-walks up to 8 times to satisfy.
   */
  grammar(opts: DrawGrammarOpts): PbExpr {
    const rootKey = resolveDictKey(opts.rootDict);
    const phraseKeys: Record<string, string> = {};
    if (opts.phrases) {
      for (const [letter, dict] of Object.entries(opts.phrases)) {
        phraseKeys[letter] = resolveDictKey(dict);
      }
    }
    const leafKeys: Record<string, string> = {};
    for (const [letter, dict] of Object.entries(opts.leaves)) {
      leafKeys[letter] = resolveDictKey(dict);
    }
    if (Object.keys(leafKeys).length === 0) {
      throw new Error("datagen: Draw.grammar requires at least one leaf dict");
    }
    const arm: PbDrawGrammar = {
      rootDict: rootKey,
      phrases: phraseKeys,
      leaves: leafKeys,
      maxLen: coerceExpr(opts.maxLen),
      minLen: opts.minLen !== undefined ? coerceExpr(opts.minLen) : undefined,
    };
    return streamDrawExpr({ oneofKind: "grammar", grammar: arm });
  },
};

/** Coerce an Expr|number|bigint into an Expr via `Expr.lit` when needed. */
function coerceExpr(v: PbExpr | number | bigint): PbExpr {
  if (typeof v === "number" || typeof v === "bigint") return Expr.lit(v);
  return v;
}

// -------- Null-helper namespace member (proto: Null on Attr) --------

export type NullSpec = PbNull;

// -------- Namespace: DrawRT (tx-time draw, iter 2) --------

/**
 * SampleableDraw is the JS-visible surface returned by every DrawRT.xxx
 * builder. Sobek binds the Go struct's Sample/Next/Seek/Reset methods
 * as camelCased JS methods via k6's FieldNameMapper.
 *
 * Concurrency: one instance per VU. Do NOT share across VUs — the
 * internal cursor is plain, not atomic.
 */
export interface SampleableDraw {
  /** Stateless sample at (seed, key). Does not touch the cursor. */
  sample(seed: number, key: number): any;
  /** Value at current cursor; advances the cursor. */
  next(): any;
  /** Set the cursor to `key` (absolute). */
  seek(key: number): void;
  /** Reset the cursor to 0. */
  reset(): void;
}

/** Coerce a Literal-arm Expr, number, or bigint to a numeric int64. */
function coerceLitInt(v: PbExpr | number | bigint): number {
  if (typeof v === "number") {
    if (!Number.isInteger(v)) {
      throw new Error(`datagen: DrawRT requires integer bound, got ${v}`);
    }
    return v;
  }
  if (typeof v === "bigint") {
    return Number(v);
  }
  const kind = v.kind;
  if (kind?.oneofKind !== "lit") {
    throw new Error("datagen: DrawRT requires literal bound, got non-literal Expr");
  }
  const val = kind.lit.value;
  if (val?.oneofKind === "int64") return Number(val.int64);
  throw new Error(`datagen: DrawRT requires int literal, got ${val?.oneofKind}`);
}

/** Coerce a Literal-arm Expr, number, or bigint to a numeric float64. */
function coerceLitFloat(v: PbExpr | number | bigint): number {
  if (typeof v === "number") return v;
  if (typeof v === "bigint") return Number(v);
  const kind = v.kind;
  if (kind?.oneofKind !== "lit") {
    throw new Error("datagen: DrawRT requires literal bound, got non-literal Expr");
  }
  const val = kind.lit.value;
  if (val?.oneofKind === "double") return val.double;
  if (val?.oneofKind === "int64") return Number(val.int64);
  throw new Error(`datagen: DrawRT requires numeric literal, got ${val?.oneofKind}`);
}

/**
 * stroppyModule is the xk6air module namespace. We defer resolution
 * until first use so datagen.ts can be imported under vitest
 * (k6/x/stroppy is absent there); tests stub the module via
 * tests/k6_stroppy_stub.ts before touching DrawRT.
 */
let stroppyModule: any | null = null;

function getStroppyModule(): any {
  if (stroppyModule !== null) return stroppyModule;
  // Require rather than import so vitest can stub the module lazily.
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  stroppyModule = require("k6/x/stroppy");
  return stroppyModule;
}

/** Override the xk6air module import — unit-test seam only. */
export function __setDrawRTStroppyModule(mod: unknown): void {
  stroppyModule = mod;
}

/**
 * Register an alphabet (AsciiRange list) with the Go handle registry.
 * Returns an opaque uint64 handle suitable for DrawRT.ascii.
 */
function registerAlphabetHandle(alphabet: ReadonlyArray<{ min: number; max: number }>): number {
  const holder: PbDrawAscii = {
    minLen: Expr.lit(0),
    maxLen: Expr.lit(0),
    alphabet: alphabet.map((r) => ({ min: r.min, max: r.max } as PbAsciiRange)),
  };
  const bin = PbDrawAscii.toBinary(holder);
  return getStroppyModule().RegisterAlphabet(bin);
}

/**
 * Register a dict body with the Go handle registry under `name`.
 * Returns an opaque uint64 handle suitable for DrawRT.dict / joint /
 * phrase. `name` additionally enters the named-dict table used by
 * DrawRT.grammar.
 */
function registerDictHandle(name: string, dict: PbDict): number {
  const bin = PbDict.toBinary(dict);
  return getStroppyModule().RegisterDict(name, bin);
}

/**
 * Resolve a DictLike to a numeric dict handle. Accepts a DictRef
 * (PbDict body or string key) and walks the pendingDicts registry to
 * recover the PbDict if given by key.
 */
function dictToHandle(d: DictLike): number {
  if (typeof d === "string") {
    const pb = pendingDicts.get(d);
    if (!pb) throw new Error(`datagen: DrawRT unknown dict key "${d}"`);
    return registerDictHandle(d, pb);
  }
  // Inline PbDict: derive a stable name from its FNV content hash so
  // duplicate registrations share a handle on the Go side (the
  // sync.Map tolerates repeat writes for the same named key).
  const key = dictKey(d);
  return registerDictHandle(key, d);
}

/** Register a grammar with the Go handle registry. */
function registerGrammarHandle(g: PbDrawGrammar): number {
  const bin = PbDrawGrammar.toBinary(g);
  return getStroppyModule().RegisterGrammar(bin);
}

/** Options accepted by DrawRT.normal. */
export interface DrawRTNormalOpts {
  screw?: number;
}

/** Options accepted by DrawRT.zipf. */
export interface DrawRTZipfOpts {
  exponent?: number;
}

/** Options accepted by DrawRT.nurand. */
export interface DrawRTNURandOpts {
  cSalt?: number | bigint;
}

/** Options accepted by DrawRT.decimal. */
export interface DrawRTDecimalOpts {
  scale: number;
}

/** Options accepted by DrawRT.dict / joint. */
export interface DrawRTDictOpts {
  weightSet?: string;
}

/** Options accepted by DrawRT.joint beyond its column argument. */
export interface DrawRTJointOpts extends DrawRTDictOpts {}

/** Options accepted by DrawRT.phrase. */
export interface DrawRTPhraseOpts {
  separator?: string;
}

/** Options accepted by DrawRT.grammar. */
export interface DrawRTGrammarOpts {
  rootDict: DictLike;
  phrases?: Record<string, DictLike>;
  leaves: Record<string, DictLike>;
  minLen?: number;
}

/**
 * DrawRT is the tx-time draw surface. Each builder resolves non-
 * literal inputs once and hands the sobek-bound Go struct back to the
 * caller, who calls .sample/.next/.seek/.reset. This path bypasses
 * expr.Eval entirely for the hot loop.
 */
export const DrawRT = {
  intUniform(
    seed: number,
    lo: PbExpr | number | bigint,
    hi: PbExpr | number | bigint,
  ): SampleableDraw {
    return getStroppyModule().NewDrawIntUniform(seed, coerceLitInt(lo), coerceLitInt(hi));
  },

  floatUniform(
    seed: number,
    lo: PbExpr | number | bigint,
    hi: PbExpr | number | bigint,
  ): SampleableDraw {
    return getStroppyModule().NewDrawFloatUniform(seed, coerceLitFloat(lo), coerceLitFloat(hi));
  },

  normal(
    seed: number,
    lo: PbExpr | number | bigint,
    hi: PbExpr | number | bigint,
    opts?: DrawRTNormalOpts,
  ): SampleableDraw {
    return getStroppyModule().NewDrawNormal(
      seed,
      coerceLitFloat(lo),
      coerceLitFloat(hi),
      opts?.screw ?? 0,
    );
  },

  zipf(
    seed: number,
    lo: PbExpr | number | bigint,
    hi: PbExpr | number | bigint,
    opts?: DrawRTZipfOpts,
  ): SampleableDraw {
    return getStroppyModule().NewDrawZipf(
      seed,
      coerceLitInt(lo),
      coerceLitInt(hi),
      opts?.exponent ?? 0,
    );
  },

  nurand(
    seed: number,
    a: Int64Like,
    x: Int64Like,
    y: Int64Like,
    opts?: DrawRTNURandOpts,
  ): SampleableDraw {
    const cSalt = opts?.cSalt ?? 0;
    return getStroppyModule().NewDrawNURand(
      seed,
      typeof a === "bigint" ? Number(a) : a,
      typeof x === "bigint" ? Number(x) : x,
      typeof y === "bigint" ? Number(y) : y,
      typeof cSalt === "bigint" ? Number(cSalt) : cSalt,
    );
  },

  bernoulli(seed: number, p: number): SampleableDraw {
    return getStroppyModule().NewDrawBernoulli(seed, p);
  },

  date(seed: number, minDate: Date, maxDate: Date): SampleableDraw {
    return getStroppyModule().NewDrawDate(seed, dateToDays(minDate), dateToDays(maxDate));
  },

  decimal(
    seed: number,
    lo: PbExpr | number | bigint,
    hi: PbExpr | number | bigint,
    opts: DrawRTDecimalOpts,
  ): SampleableDraw {
    return getStroppyModule().NewDrawDecimal(
      seed,
      coerceLitFloat(lo),
      coerceLitFloat(hi),
      opts.scale,
    );
  },

  ascii(
    seed: number,
    minLen: number,
    maxLen: number,
    alphabet?: ReadonlyArray<{ min: number; max: number }>,
  ): SampleableDraw {
    const handle = registerAlphabetHandle(alphabet ?? Alphabet.en);
    return getStroppyModule().NewDrawASCII(seed, minLen, maxLen, handle);
  },

  dict(seed: number, d: DictLike, opts?: DrawRTDictOpts): SampleableDraw {
    return getStroppyModule().NewDrawDict(seed, dictToHandle(d), opts?.weightSet ?? "");
  },

  joint(seed: number, d: DictLike, column: string, opts?: DrawRTJointOpts): SampleableDraw {
    return getStroppyModule().NewDrawJoint(
      seed,
      dictToHandle(d),
      column,
      opts?.weightSet ?? "",
    );
  },

  phrase(
    seed: number,
    vocab: DictLike,
    minW: number,
    maxW: number,
    opts?: DrawRTPhraseOpts,
  ): SampleableDraw {
    return getStroppyModule().NewDrawPhrase(
      seed,
      dictToHandle(vocab),
      minW,
      maxW,
      opts?.separator ?? " ",
    );
  },

  grammar(seed: number, maxLen: number, opts: DrawRTGrammarOpts): SampleableDraw {
    // Register the root + phrase + leaf dicts under stable names so
    // the Go grammar walker can resolve them by name.
    const rootKey = resolveDictKey(opts.rootDict);
    const rootPb = pendingDicts.get(rootKey);
    if (!rootPb) throw new Error(`datagen: DrawRT.grammar unknown rootDict "${rootKey}"`);
    registerDictHandle(rootKey, rootPb);

    const phraseKeys: Record<string, string> = {};
    if (opts.phrases) {
      for (const [letter, d] of Object.entries(opts.phrases)) {
        const k = resolveDictKey(d);
        const pb = pendingDicts.get(k);
        if (!pb) throw new Error(`datagen: DrawRT.grammar unknown phrase dict "${k}"`);
        registerDictHandle(k, pb);
        phraseKeys[letter] = k;
      }
    }

    const leafKeys: Record<string, string> = {};
    for (const [letter, d] of Object.entries(opts.leaves)) {
      const k = resolveDictKey(d);
      const pb = pendingDicts.get(k);
      if (!pb) throw new Error(`datagen: DrawRT.grammar unknown leaf dict "${k}"`);
      registerDictHandle(k, pb);
      leafKeys[letter] = k;
    }

    const grammarPb: PbDrawGrammar = {
      rootDict: rootKey,
      phrases: phraseKeys,
      leaves: leafKeys,
      maxLen: Expr.lit(maxLen),
      minLen: opts.minLen !== undefined ? Expr.lit(opts.minLen) : undefined,
    };
    const handle = registerGrammarHandle(grammarPb);

    return getStroppyModule().NewDrawGrammar(seed, handle, opts.minLen ?? 0, maxLen);
  },
};

// -------- Convenience re-exports of enums commonly used in workload code --------

export { InsertMethod, RowIndex_Kind };

// -------- Type re-exports that workloads may reference --------

export type { PbExpr as Expression };
export type { PbInsertSpec as InsertSpec };
export type { PbDict as DictBody };
