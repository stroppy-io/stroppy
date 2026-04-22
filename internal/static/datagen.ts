/// <reference lib="es2020.bigint" />
/**
 * datagen.ts — Ergonomic TS wrapper over the generated stroppy.datagen proto
 * types. Workload authors compose `InsertSpec` values through five namespaces:
 * `Rel`, `Attr`, `Expr`, `Dict`, `std`. `Draw` is reserved for Stage D.
 *
 * The wrapper hides the oneof-kind boilerplate, converts int64-typed fields
 * between `number`/`bigint` and the protobuf-ts wire form (string), and
 * deduplicates Dict bodies by content so equal-content dicts collapse to a
 * single entry in `InsertSpec.dicts`.
 */
import {
  Attr as PbAttr,
  BinOp_Op,
  BlockRef as PbBlockRef,
  BlockSlot as PbBlockSlot,
  Call as PbCall,
  Cohort as PbCohort,
  Degree as PbDegree,
  DictRow as PbDictRow,
  Dict as PbDict,
  DictAt as PbDictAt,
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
    return call("std.hash_mod", [n, k]);
  },

  /** Deterministic UUID v4 derived from a 64-bit seed. */
  uuidSeeded(seed: PbExpr): PbExpr {
    return call("std.uuid_seeded", [seed]);
  },

  /** Convert epoch-days into a date scalar (YYYY-MM-DD on SQL side). */
  daysToDate(days: PbExpr): PbExpr {
    return call("std.days_to_date", [days]);
  },

  /** Convert a date scalar into epoch-days. */
  dateToDays(t: PbExpr): PbExpr {
    return call("std.date_to_days", [t]);
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
    return call("std.to_string", [x]);
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

export const Dict = {
  values: dictValues,
  weighted: dictWeighted,
};

/** Anything accepted where a Dict reference is expected. */
export type DictRef = PbDict | string;

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

export const Rel = {
  table: relTable,
  relationship: relRelationship,
  side: relSide,
  lookupPop: relLookupPop,
  scd2: relSCD2,
};

// -------- Namespace: Draw (reserved) --------

/**
 * Draw is the stream-draw namespace. Populated in Stage D (StreamDraw
 * primitives: intUniform, ascii, bernoulli, zipf, nurand, date, decimal,
 * phrase, dict, joint). Kept here so workloads can import the five core
 * namespaces plus Draw from a single module once Stage D lands.
 */
export const Draw: Record<string, never> = {};

// -------- Null-helper namespace member (proto: Null on Attr) --------

export type NullSpec = PbNull;

// -------- Convenience re-exports of enums commonly used in workload code --------

export { InsertMethod, RowIndex_Kind };

// -------- Type re-exports that workloads may reference --------

export type { PbExpr as Expression };
export type { PbInsertSpec as InsertSpec };
export type { PbDict as DictBody };
