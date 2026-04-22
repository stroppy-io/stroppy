import { describe, it, expect } from "vitest";
import {
  Alphabet,
  Attr,
  Deg,
  Dict,
  Draw,
  Expr,
  Rel,
  Strat,
  std,
  InsertMethod,
  RowIndex_Kind,
} from "../datagen.ts";

describe("Rel.table", () => {
  it("infers columnOrder from attrs insertion order", () => {
    const spec = Rel.table("nations", {
      size: 25,
      seed: 42,
      attrs: {
        n_nationkey: Attr.rowIndex(),
        n_name: Expr.lit("ALGERIA"),
        n_regionkey: Expr.lit(0),
      },
    });
    expect(spec.source?.columnOrder).toEqual([
      "n_nationkey",
      "n_name",
      "n_regionkey",
    ]);
    expect(spec.source?.attrs.map((a) => a.name)).toEqual([
      "n_nationkey",
      "n_name",
      "n_regionkey",
    ]);
    expect(spec.table).toBe("nations");
    expect(spec.seed).toBe("42");
    expect(spec.source?.population?.size).toBe("25");
    expect(spec.method).toBe(InsertMethod.PLAIN_QUERY);
  });

  it("honors explicit columnOrder override", () => {
    const spec = Rel.table("t", {
      size: 1,
      attrs: { a: Expr.lit(1), b: Expr.lit(2) },
      columnOrder: ["b", "a"],
    });
    expect(spec.source?.columnOrder).toEqual(["b", "a"]);
  });

  it("rejects columnOrder with unknown or missing attrs", () => {
    expect(() =>
      Rel.table("t", {
        size: 1,
        attrs: { a: Expr.lit(1) },
        columnOrder: ["a", "b"],
      }),
    ).toThrow();
    expect(() =>
      Rel.table("t", {
        size: 1,
        attrs: { a: Expr.lit(1), b: Expr.lit(2) },
        columnOrder: ["a", "a"],
      }),
    ).toThrow();
  });

  it("accepts bigint size", () => {
    const spec = Rel.table("t", {
      size: BigInt("9999999999"),
      attrs: { a: Attr.rowId() },
    });
    expect(spec.source?.population?.size).toBe("9999999999");
  });
});

describe("Dict dedup", () => {
  it("collapses two attrs using equal-content dicts to one entry", () => {
    const d1 = Dict.values(["A", "B", "C"]);
    const d2 = Dict.values(["A", "B", "C"]);
    const spec = Rel.table("t", {
      size: 10,
      attrs: {
        col1: Attr.dictAt(d1, Attr.rowIndex()),
        col2: Attr.dictAt(d2, Attr.rowIndex()),
      },
    });
    const keys = Object.keys(spec.dicts);
    expect(keys).toHaveLength(1);
    const key = keys[0];
    expect(key).toMatch(/^d_[0-9a-f]{16}$/);

    // Both attrs must reference the same key.
    const attr1 = spec.source?.attrs[0].expr!;
    const attr2 = spec.source?.attrs[1].expr!;
    if (attr1.kind.oneofKind !== "dictAt" || attr2.kind.oneofKind !== "dictAt") {
      throw new Error("expected dictAt arms");
    }
    expect(attr1.kind.dictAt.dictKey).toBe(key);
    expect(attr2.kind.dictAt.dictKey).toBe(key);
  });

  it("keeps distinct dict bodies under distinct keys", () => {
    const spec = Rel.table("t", {
      size: 10,
      attrs: {
        col1: Attr.dictAt(Dict.values(["A", "B"]), Attr.rowIndex()),
        col2: Attr.dictAt(Dict.values(["X", "Y"]), Attr.rowIndex()),
      },
    });
    expect(Object.keys(spec.dicts)).toHaveLength(2);
  });

  it("weighted dict carries a default weight set", () => {
    const d = Dict.weighted(["A", "B"], [1, 3]);
    expect(d.weightSets).toEqual([""]);
    expect(d.rows[0].weights).toEqual(["1"]);
    expect(d.rows[1].weights).toEqual(["3"]);
  });
});

describe("Expr.lit oneof dispatch", () => {
  it("routes integer number to int64", () => {
    const e = Expr.lit(5);
    if (e.kind.oneofKind !== "lit") throw new Error("not a lit");
    expect(e.kind.lit.value.oneofKind).toBe("int64");
    if (e.kind.lit.value.oneofKind === "int64") {
      expect(e.kind.lit.value.int64).toBe("5");
    }
  });

  it("routes bigint to int64", () => {
    const e = Expr.lit(BigInt("9007199254740993"));
    if (e.kind.oneofKind !== "lit") throw new Error("not a lit");
    if (e.kind.lit.value.oneofKind === "int64") {
      expect(e.kind.lit.value.int64).toBe("9007199254740993");
    } else {
      throw new Error("expected int64 arm");
    }
  });

  it("routes fractional number to double", () => {
    const e = Expr.lit(5.5);
    if (e.kind.oneofKind !== "lit") throw new Error("not a lit");
    expect(e.kind.lit.value.oneofKind).toBe("double");
    if (e.kind.lit.value.oneofKind === "double") {
      expect(e.kind.lit.value.double).toBe(5.5);
    }
  });

  it("routes string, boolean, date", () => {
    const s = Expr.lit("hi");
    if (s.kind.oneofKind === "lit" && s.kind.lit.value.oneofKind === "string") {
      expect(s.kind.lit.value.string).toBe("hi");
    } else {
      throw new Error("expected string lit");
    }

    const b = Expr.lit(true);
    if (b.kind.oneofKind === "lit" && b.kind.lit.value.oneofKind === "bool") {
      expect(b.kind.lit.value.bool).toBe(true);
    } else {
      throw new Error("expected bool lit");
    }

    const d = Expr.lit(new Date("1970-01-11T00:00:00Z"));
    if (d.kind.oneofKind === "lit" && d.kind.lit.value.oneofKind === "int64") {
      expect(d.kind.lit.value.int64).toBe("10");
    } else {
      throw new Error("expected date → int64 days lit");
    }
  });
});

describe("Rel.relationship / Rel.side", () => {
  it("Rel.relationship with two sides builds the Relationship proto", () => {
    const parent = Rel.side("orders", {
      degree: Deg.fixed(1),
      strategy: Strat.sequential(),
    });
    const child = Rel.side("lineitem", {
      degree: Deg.fixed(7),
      strategy: Strat.sequential(),
    });
    const rel = Rel.relationship("orders_lineitem", [parent, child]);
    expect(rel.name).toBe("orders_lineitem");
    expect(rel.sides).toHaveLength(2);
    expect(rel.sides[0].population).toBe("orders");
    expect(rel.sides[1].population).toBe("lineitem");
  });

  it("Rel.relationship rejects fewer than two sides", () => {
    const s = Rel.side("only", {
      degree: Deg.fixed(1),
      strategy: Strat.sequential(),
    });
    expect(() => Rel.relationship("r", [s])).toThrow();
  });

  it("Rel.side with Deg.fixed + Strat.sequential + blockSlots", () => {
    const side = Rel.side("lineitem", {
      degree: Deg.fixed(3),
      strategy: Strat.sequential(),
      blockSlots: {
        o_orderkey: Attr.rowIndex(),
        o_custkey: Expr.lit(BigInt(42)),
      },
    });
    expect(side.population).toBe("lineitem");
    if (side.degree?.kind.oneofKind !== "fixed") {
      throw new Error("expected fixed degree");
    }
    expect(side.degree.kind.fixed.count).toBe("3");
    if (side.strategy?.kind.oneofKind !== "sequential") {
      throw new Error("expected sequential strategy");
    }
    expect(side.blockSlots.map((s) => s.name)).toEqual([
      "o_orderkey",
      "o_custkey",
    ]);
    const second = side.blockSlots[1].expr!;
    if (second.kind.oneofKind !== "lit" || second.kind.lit.value.oneofKind !== "int64") {
      throw new Error("expected int64 lit in block slot");
    }
    expect(second.kind.lit.value.int64).toBe("42");
  });

  it("Deg.uniform and Strat.hash/equitable build correct arms", () => {
    const d = Deg.uniform(1, 7);
    if (d.kind.oneofKind !== "uniform") throw new Error("expected uniform");
    expect(d.kind.uniform.min).toBe("1");
    expect(d.kind.uniform.max).toBe("7");

    expect(Strat.hash().kind.oneofKind).toBe("hash");
    expect(Strat.equitable().kind.oneofKind).toBe("equitable");
  });
});

describe("Rel.lookupPop", () => {
  it("infers columnOrder from attrs key order and defaults pure=true", () => {
    const lp = Rel.lookupPop({
      name: "region",
      size: 5,
      attrs: {
        r_regionkey: Attr.rowIndex(),
        r_name: Expr.lit("AFRICA"),
        r_comment: Expr.lit("lorem"),
      },
    });
    expect(lp.population?.name).toBe("region");
    expect(lp.population?.size).toBe("5");
    expect(lp.population?.pure).toBe(true);
    expect(lp.columnOrder).toEqual(["r_regionkey", "r_name", "r_comment"]);
    expect(lp.attrs.map((a) => a.name)).toEqual([
      "r_regionkey",
      "r_name",
      "r_comment",
    ]);
  });

  it("honors explicit pure=false and attaches null spec", () => {
    const lp = Rel.lookupPop({
      name: "t",
      size: BigInt(10),
      pure: false,
      attrs: {
        a: { expr: Expr.lit(1), null: { rate: 0.5, seedSalt: "7" } },
      },
    });
    expect(lp.population?.pure).toBe(false);
    expect(lp.attrs[0].null?.rate).toBeCloseTo(0.5);
    expect(lp.attrs[0].null?.seedSalt).toBe("7");
  });
});

describe("Attr.lookup / Attr.blockRef / Expr.blockRef", () => {
  it("Attr.lookup emits a Lookup arm with target_pop, attr_name, entity_index", () => {
    const e = Attr.lookup("region", "r_name", Expr.col("r_regionkey"));
    if (e.kind.oneofKind !== "lookup") throw new Error("expected lookup");
    expect(e.kind.lookup.targetPop).toBe("region");
    expect(e.kind.lookup.attrName).toBe("r_name");
    if (e.kind.lookup.entityIndex?.kind.oneofKind !== "col") {
      throw new Error("expected col expr for entity_index");
    }
    expect(e.kind.lookup.entityIndex.kind.col.name).toBe("r_regionkey");
  });

  it("Attr.blockRef and Expr.blockRef emit BlockRef arms with the slot name", () => {
    const a = Attr.blockRef("o_orderkey");
    const b = Expr.blockRef("o_orderkey");
    if (a.kind.oneofKind !== "blockRef") throw new Error("expected blockRef");
    if (b.kind.oneofKind !== "blockRef") throw new Error("expected blockRef");
    expect(a.kind.blockRef.slot).toBe("o_orderkey");
    expect(b.kind.blockRef.slot).toBe("o_orderkey");
  });

  it("Attr.lookup rejects empty names", () => {
    expect(() => Attr.lookup("", "a", Expr.lit(0))).toThrow();
    expect(() => Attr.lookup("p", "", Expr.lit(0))).toThrow();
  });
});

describe("Rel.table with relationships / iter / lookupPops", () => {
  it("emits RelSource fields populated from opts", () => {
    const lp = Rel.lookupPop({
      name: "region",
      size: 5,
      attrs: {
        r_regionkey: Attr.rowIndex(),
        r_name: Expr.lit("AFRICA"),
      },
    });
    const parent = Rel.side("orders", {
      degree: Deg.fixed(1),
      strategy: Strat.sequential(),
    });
    const child = Rel.side("lineitem", {
      degree: Deg.fixed(7),
      strategy: Strat.sequential(),
      blockSlots: { o_orderkey: Attr.rowIndex() },
    });
    const rel = Rel.relationship("orders_lineitem", [parent, child]);

    const spec = Rel.table("lineitem", {
      size: 1,
      iter: "orders_lineitem",
      relationships: [rel],
      lookupPops: [lp],
      attrs: {
        l_orderkey: Expr.blockRef("o_orderkey"),
        l_regionkey: Attr.lookup("region", "r_regionkey", Attr.rowIndex()),
      },
    });

    expect(spec.source?.iter).toBe("orders_lineitem");
    expect(spec.source?.relationships).toHaveLength(1);
    expect(spec.source?.relationships[0].name).toBe("orders_lineitem");
    expect(spec.source?.lookupPops).toHaveLength(1);
    expect(spec.source?.lookupPops[0].population?.name).toBe("region");
    expect(spec.source?.lookupPops[0].population?.pure).toBe(true);
  });
});

describe("Dict dedup with lookupPops", () => {
  it("dedupes dicts referenced by both table attrs and lookup-pop attrs", () => {
    const shared = Dict.values(["A", "B", "C"]);
    const lp = Rel.lookupPop({
      name: "shared_lookup",
      size: 3,
      attrs: {
        s_key: Attr.rowIndex(),
        s_label: Attr.dictAt(shared, Attr.rowIndex()),
      },
    });
    const spec = Rel.table("main", {
      size: 10,
      lookupPops: [lp],
      attrs: {
        m_idx: Attr.rowIndex(),
        m_label: Attr.dictAt(shared, Attr.rowIndex()),
      },
    });
    const keys = Object.keys(spec.dicts);
    expect(keys).toHaveLength(1);
    const key = keys[0];
    // Both the table attr and the lookup-pop attr resolve to the same key.
    const tableAttr = spec.source?.attrs[1].expr!;
    if (tableAttr.kind.oneofKind !== "dictAt") throw new Error("expected dictAt");
    expect(tableAttr.kind.dictAt.dictKey).toBe(key);

    const lpAttr = spec.source?.lookupPops[0].attrs[1].expr!;
    if (lpAttr.kind.oneofKind !== "dictAt") throw new Error("expected dictAt");
    expect(lpAttr.kind.dictAt.dictKey).toBe(key);
  });
});

describe("Rel.scd2", () => {
  it("emits the SCD2 shape from options", () => {
    const s = Rel.scd2({
      startCol: "valid_from",
      endCol: "valid_to",
      boundary: Expr.lit(5),
      historicalStart: Expr.lit("1900-01-01"),
      historicalEnd: Expr.lit("1999-12-31"),
      currentStart: Expr.lit("2000-01-01"),
      currentEnd: Expr.lit("9999-12-31"),
    });
    expect(s.startCol).toBe("valid_from");
    expect(s.endCol).toBe("valid_to");
    if (s.boundary?.kind.oneofKind !== "lit") throw new Error("expected lit");
    if (s.boundary.kind.lit.value.oneofKind === "int64") {
      expect(s.boundary.kind.lit.value.int64).toBe("5");
    } else {
      throw new Error("boundary should be int64");
    }
    expect(s.historicalStart).toBeDefined();
    expect(s.historicalEnd).toBeDefined();
    expect(s.currentStart).toBeDefined();
    expect(s.currentEnd).toBeDefined();
  });

  it("allows omitting currentEnd", () => {
    const s = Rel.scd2({
      startCol: "s",
      endCol: "e",
      boundary: Expr.lit(1),
      historicalStart: Expr.lit("h"),
      historicalEnd: Expr.lit("h"),
      currentStart: Expr.lit("c"),
    });
    expect(s.currentEnd).toBeUndefined();
  });

  it("rejects equal startCol and endCol", () => {
    expect(() =>
      Rel.scd2({
        startCol: "x",
        endCol: "x",
        boundary: Expr.lit(0),
        historicalStart: Expr.lit("h"),
        historicalEnd: Expr.lit("h"),
        currentStart: Expr.lit("c"),
      }),
    ).toThrow();
  });
});

describe("Rel.table with scd2", () => {
  it("auto-appends start_col and end_col to columnOrder", () => {
    const s = Rel.scd2({
      startCol: "valid_from",
      endCol: "valid_to",
      boundary: Expr.lit(5),
      historicalStart: Expr.lit("1900-01-01"),
      historicalEnd: Expr.lit("1999-12-31"),
      currentStart: Expr.lit("2000-01-01"),
    });
    const spec = Rel.table("item", {
      size: 10,
      attrs: {
        i_id: Attr.rowId(),
        i_name: Expr.lit("widget"),
      },
      scd2: s,
    });
    expect(spec.source?.columnOrder).toEqual([
      "i_id",
      "i_name",
      "valid_from",
      "valid_to",
    ]);
    expect(spec.source?.scd2?.startCol).toBe("valid_from");
    expect(spec.source?.scd2?.endCol).toBe("valid_to");
  });

  it("rejects a scd2 column that collides with an attr name", () => {
    const s = Rel.scd2({
      startCol: "a",
      endCol: "valid_to",
      boundary: Expr.lit(1),
      historicalStart: Expr.lit("h"),
      historicalEnd: Expr.lit("h"),
      currentStart: Expr.lit("c"),
    });
    expect(() =>
      Rel.table("t", {
        size: 1,
        attrs: { a: Expr.lit(1) },
        scd2: s,
      }),
    ).toThrow();
  });

  it("honors an explicit columnOrder that mixes attrs and scd2 columns", () => {
    const s = Rel.scd2({
      startCol: "vf",
      endCol: "vt",
      boundary: Expr.lit(1),
      historicalStart: Expr.lit("h"),
      historicalEnd: Expr.lit("h"),
      currentStart: Expr.lit("c"),
    });
    const spec = Rel.table("t", {
      size: 1,
      attrs: { a: Expr.lit(1), b: Expr.lit(2) },
      columnOrder: ["vf", "a", "vt", "b"],
      scd2: s,
    });
    expect(spec.source?.columnOrder).toEqual(["vf", "a", "vt", "b"]);
  });
});

describe("std.* wrappers", () => {
  it("std.format builds a Call with std.format and the given args", () => {
    const e = std.format(Expr.lit("%02d"), Expr.lit(7));
    if (e.kind.oneofKind !== "call") throw new Error("not a call");
    expect(e.kind.call.func).toBe("std.format");
    expect(e.kind.call.args).toHaveLength(2);
  });

  it("Attr.rowId = rowIndex() + 1", () => {
    const e = Attr.rowId();
    if (e.kind.oneofKind !== "binOp") throw new Error("not a binOp");
    const a = e.kind.binOp.a;
    const b = e.kind.binOp.b;
    if (a?.kind.oneofKind !== "rowIndex") throw new Error("expected rowIndex");
    expect(a.kind.rowIndex.kind).toBe(RowIndex_Kind.UNSPECIFIED);
    if (b?.kind.oneofKind !== "lit") throw new Error("expected lit");
    if (b.kind.lit.value.oneofKind === "int64") {
      expect(b.kind.lit.value.int64).toBe("1");
    } else {
      throw new Error("expected int64 arm on +1");
    }
  });
});

// Helper to unwrap StreamDraw Expr and assert arm kind.
function unwrapDraw<K extends string>(
  e: ReturnType<typeof Draw.intUniform>,
  kind: K,
) {
  if (e.kind.oneofKind !== "streamDraw") throw new Error("not a streamDraw");
  const arm = e.kind.streamDraw.draw;
  if (arm.oneofKind !== kind) {
    throw new Error(`expected draw arm ${kind}, got ${arm.oneofKind}`);
  }
  expect(e.kind.streamDraw.streamId).toBe(0);
  return arm;
}

describe("Draw primitives", () => {
  it("Draw.intUniform emits a StreamDraw.int_uniform arm", () => {
    const e = Draw.intUniform({ min: Expr.lit(0), max: Expr.lit(99) });
    const arm = unwrapDraw(e, "intUniform");
    if (arm.oneofKind !== "intUniform") throw new Error("narrow");
    expect(arm.intUniform.min).toBeDefined();
    expect(arm.intUniform.max).toBeDefined();
  });

  it("Draw.floatUniform emits float_uniform arm", () => {
    const e = Draw.floatUniform({ min: Expr.lit(0.1), max: Expr.lit(0.9) });
    unwrapDraw(e, "floatUniform");
  });

  it("Draw.normal carries screw (0 defaults to runtime default)", () => {
    const e = Draw.normal({
      min: Expr.lit(0),
      max: Expr.lit(100),
      screw: 2.5,
    });
    const arm = unwrapDraw(e, "normal");
    if (arm.oneofKind !== "normal") throw new Error("narrow");
    expect(arm.normal.screw).toBeCloseTo(2.5);

    const eDef = Draw.normal({ min: Expr.lit(0), max: Expr.lit(100) });
    const armDef = unwrapDraw(eDef, "normal");
    if (armDef.oneofKind !== "normal") throw new Error("narrow");
    expect(armDef.normal.screw).toBe(0);
  });

  it("Draw.zipf carries exponent", () => {
    const e = Draw.zipf({
      min: Expr.lit(1),
      max: Expr.lit(1000),
      exponent: 1.3,
    });
    const arm = unwrapDraw(e, "zipf");
    if (arm.oneofKind !== "zipf") throw new Error("narrow");
    expect(arm.zipf.exponent).toBeCloseTo(1.3);
  });

  it("Draw.nurand stringifies a/x/y and cSalt (defaults to 0)", () => {
    const e = Draw.nurand({ a: 255, x: 1, y: 100, cSalt: 0xabcd });
    const arm = unwrapDraw(e, "nurand");
    if (arm.oneofKind !== "nurand") throw new Error("narrow");
    expect(arm.nurand.a).toBe("255");
    expect(arm.nurand.x).toBe("1");
    expect(arm.nurand.y).toBe("100");
    expect(arm.nurand.cSalt).toBe(BigInt(0xabcd).toString());

    const eDef = Draw.nurand({ a: 255, x: 1, y: 100 });
    const armDef = unwrapDraw(eDef, "nurand");
    if (armDef.oneofKind !== "nurand") throw new Error("narrow");
    expect(armDef.nurand.cSalt).toBe("0");
  });

  it("Draw.bernoulli carries p", () => {
    const e = Draw.bernoulli({ p: 0.3 });
    const arm = unwrapDraw(e, "bernoulli");
    if (arm.oneofKind !== "bernoulli") throw new Error("narrow");
    expect(arm.bernoulli.p).toBeCloseTo(0.3);
  });

  it("Draw.date converts Dates to inclusive epoch-day bounds", () => {
    const e = Draw.date({
      minDate: new Date("1970-01-01T00:00:00Z"),
      maxDate: new Date("1970-01-11T00:00:00Z"),
    });
    const arm = unwrapDraw(e, "date");
    if (arm.oneofKind !== "date") throw new Error("narrow");
    expect(arm.date.minDaysEpoch).toBe("0");
    expect(arm.date.maxDaysEpoch).toBe("10");
  });

  it("Draw.decimal carries min/max/scale", () => {
    const e = Draw.decimal({ min: Expr.lit(1.0), max: Expr.lit(999.99), scale: 2 });
    const arm = unwrapDraw(e, "decimal");
    if (arm.oneofKind !== "decimal") throw new Error("narrow");
    expect(arm.decimal.scale).toBe(2);
  });

  it("Draw.decimal rejects negative or non-integer scale", () => {
    expect(() => Draw.decimal({ min: Expr.lit(0), max: Expr.lit(1), scale: -1 })).toThrow();
    expect(() => Draw.decimal({ min: Expr.lit(0), max: Expr.lit(1), scale: 1.5 })).toThrow();
  });

  it("Draw.ascii defaults to Alphabet.en and copies ranges", () => {
    const eDef = Draw.ascii({ min: Expr.lit(3), max: Expr.lit(5) });
    const armDef = unwrapDraw(eDef, "ascii");
    if (armDef.oneofKind !== "ascii") throw new Error("narrow");
    expect(armDef.ascii.alphabet).toHaveLength(Alphabet.en.length);
    expect(armDef.ascii.alphabet[0]).toEqual({ min: 65, max: 90 });

    const eNum = Draw.ascii({ min: Expr.lit(3), max: Expr.lit(5), alphabet: Alphabet.num });
    const armNum = unwrapDraw(eNum, "ascii");
    if (armNum.oneofKind !== "ascii") throw new Error("narrow");
    expect(armNum.ascii.alphabet).toEqual([{ min: 48, max: 57 }]);
  });

  it("Draw.phrase registers vocab dict and carries separator default", () => {
    const vocab = Dict.values(["alpha", "beta", "gamma"]);
    const e = Draw.phrase({
      vocab,
      minWords: Expr.lit(1),
      maxWords: Expr.lit(3),
    });
    const arm = unwrapDraw(e, "phrase");
    if (arm.oneofKind !== "phrase") throw new Error("narrow");
    expect(arm.phrase.vocabKey).toMatch(/^d_[0-9a-f]{16}$/);
    expect(arm.phrase.separator).toBe(" ");
  });

  it("Draw.dict wraps a DictLike with optional weightSet", () => {
    const d = Dict.weighted(["A", "B"], [1, 3]);
    const e = Draw.dict(d, { weightSet: "" });
    const arm = unwrapDraw(e, "dict");
    if (arm.oneofKind !== "dict") throw new Error("narrow");
    expect(arm.dict.dictKey).toMatch(/^d_[0-9a-f]{16}$/);
    expect(arm.dict.weightSet).toBe("");
  });

  it("Draw.joint requires a column name and carries weightSet+tupleScope", () => {
    const d = Dict.joint(
      ["marital", "edu"],
      [
        { values: ["S", "COLLEGE"] },
        { values: ["M", "HIGH_SCHOOL"] },
      ],
    );
    const e = Draw.joint(d, "marital", { weightSet: "default", tupleScope: 7 });
    const arm = unwrapDraw(e, "joint");
    if (arm.oneofKind !== "joint") throw new Error("narrow");
    expect(arm.joint.column).toBe("marital");
    expect(arm.joint.weightSet).toBe("default");
    expect(arm.joint.tupleScope).toBe(7);

    expect(() => Draw.joint(d, "")).toThrow();
  });
});

describe("Alphabet constants", () => {
  it("en covers A-Z and a-z", () => {
    expect(Alphabet.en).toEqual([
      { min: 65, max: 90 },
      { min: 97, max: 122 },
    ]);
  });

  it("num covers 0-9", () => {
    expect(Alphabet.num).toEqual([{ min: 48, max: 57 }]);
  });

  it("enNum stacks letters + digits", () => {
    expect(Alphabet.enNum).toEqual([
      { min: 65, max: 90 },
      { min: 97, max: 122 },
      { min: 48, max: 57 },
    ]);
  });

  it("enUpper is just A-Z", () => {
    expect(Alphabet.enUpper).toEqual([{ min: 65, max: 90 }]);
  });

  it("enSpc and enNumSpc include the [32, 33] space range", () => {
    expect(Alphabet.enSpc).toEqual([
      { min: 65, max: 90 },
      { min: 97, max: 122 },
      { min: 32, max: 33 },
    ]);
    expect(Alphabet.enNumSpc[Alphabet.enNumSpc.length - 1]).toEqual({
      min: 32,
      max: 33,
    });
  });

  it("ascii covers printable [32, 126]", () => {
    expect(Alphabet.ascii).toEqual([{ min: 32, max: 126 }]);
  });
});

describe("Dict.multiWeighted / Dict.joint / Dict.jointWeighted", () => {
  it("multiWeighted preserves profile names and per-row weight tuples", () => {
    const d = Dict.multiWeighted(
      ["def", "wrong", "late"],
      { default: [30, 20, 10], premium: [5, 40, 5] },
    );
    expect(d.columns).toEqual([]);
    expect(d.weightSets).toEqual(["default", "premium"]);
    expect(d.rows).toHaveLength(3);
    expect(d.rows[0].values).toEqual(["def"]);
    expect(d.rows[0].weights).toEqual(["30", "5"]);
    expect(d.rows[2].weights).toEqual(["10", "5"]);
  });

  it("multiWeighted rejects mismatched profile lengths", () => {
    expect(() =>
      Dict.multiWeighted(["a", "b"], { only: [1] }),
    ).toThrow();
  });

  it("joint produces uniform dict when no row has weights", () => {
    const d = Dict.joint(
      ["nation", "region"],
      [
        { values: ["ALGERIA", "0"] },
        { values: ["ARGENTINA", "1"] },
      ],
    );
    expect(d.columns).toEqual(["nation", "region"]);
    expect(d.weightSets).toEqual([]);
    expect(d.rows[0].values).toEqual(["ALGERIA", "0"]);
    expect(d.rows[0].weights).toEqual([]);
  });

  it("joint adds default weight-set when any row is weighted", () => {
    const d = Dict.joint(
      ["a", "b"],
      [
        { values: ["x", "y"], weights: [7] },
        { values: ["p", "q"] },
      ],
    );
    expect(d.weightSets).toEqual([""]);
    expect(d.rows[0].weights).toEqual(["7"]);
    expect(d.rows[1].weights).toEqual(["0"]);
  });

  it("joint validates row width", () => {
    expect(() =>
      Dict.joint(["a", "b"], [{ values: ["only"] }]),
    ).toThrow();
  });

  it("jointWeighted requires parallel weight tuples per row", () => {
    const d = Dict.jointWeighted(
      ["marital", "edu"],
      ["default", "premium"],
      [
        { values: ["S", "COLLEGE"], weights: [100, 40] },
        { values: ["M", "HIGH_SCHOOL"], weights: [80, 60] },
      ],
    );
    expect(d.columns).toEqual(["marital", "edu"]);
    expect(d.weightSets).toEqual(["default", "premium"]);
    expect(d.rows[0].weights).toEqual(["100", "40"]);
    expect(d.rows[1].weights).toEqual(["80", "60"]);

    expect(() =>
      Dict.jointWeighted(
        ["a"],
        ["default"],
        [{ values: ["x"], weights: [1, 2] }],
      ),
    ).toThrow();
  });
});

describe("Dict.fromJson", () => {
  it("round-trips a dstparse-shaped scalar dict", () => {
    const json = {
      rows: [
        { values: ["SMALL"] },
        { values: ["LARGE"] },
      ],
    };
    const d = Dict.fromJson(json);
    expect(d.columns).toEqual([]);
    expect(d.weightSets).toEqual([]);
    expect(d.rows.map((r) => r.values[0])).toEqual(["SMALL", "LARGE"]);
  });

  it("round-trips a multi-column multi-profile joint dict", () => {
    const json = {
      columns: ["marital", "edu"],
      weight_sets: ["default", "premium"],
      rows: [
        { values: ["S", "COLLEGE"], weights: [100, 40] },
        { values: ["M", "HIGH_SCHOOL"], weights: [80, 60] },
      ],
    };
    const d = Dict.fromJson(json);
    expect(d.columns).toEqual(["marital", "edu"]);
    expect(d.weightSets).toEqual(["default", "premium"]);
    expect(d.rows[0].values).toEqual(["S", "COLLEGE"]);
    expect(d.rows[0].weights).toEqual(["100", "40"]);
  });

  it("enforces parallel weight counts when weight_sets declared", () => {
    const json = {
      columns: ["a"],
      weight_sets: ["x", "y"],
      rows: [{ values: ["v"], weights: [1] }],
    };
    expect(() => Dict.fromJson(json)).toThrow();
  });

  it("coerces numeric values to strings", () => {
    const json = {
      rows: [{ values: [42] }, { values: [BigInt(123)] }],
    };
    const d = Dict.fromJson(json);
    expect(d.rows[0].values).toEqual(["42"]);
    expect(d.rows[1].values).toEqual(["123"]);
  });
});

describe("Attr.cohortDraw / Attr.cohortLive / Rel.cohort", () => {
  it("Rel.cohort packs entity bounds, size, and persistence fields", () => {
    const c = Rel.cohort({
      name: "hot",
      cohortSize: 20,
      entityMin: 1,
      entityMax: 500,
      activeEvery: 3,
      persistenceMod: 100,
      persistenceRatio: 0.25,
      seedSalt: 0xdeadbeef,
    });
    expect(c.name).toBe("hot");
    expect(c.cohortSize).toBe("20");
    expect(c.entityMin).toBe("1");
    expect(c.entityMax).toBe("500");
    expect(c.activeEvery).toBe("3");
    expect(c.persistenceMod).toBe("100");
    expect(c.persistenceRatio).toBeCloseTo(0.25);
    expect(c.seedSalt).toBe(BigInt(0xdeadbeef).toString());
  });

  it("Attr.cohortDraw emits a cohort_draw arm with slot + bucketKey override", () => {
    const e = Attr.cohortDraw("hot", Expr.lit(2), Expr.col("bucket"));
    if (e.kind.oneofKind !== "cohortDraw") throw new Error("not a cohortDraw");
    expect(e.kind.cohortDraw.name).toBe("hot");
    expect(e.kind.cohortDraw.slot).toBeDefined();
    expect(e.kind.cohortDraw.bucketKey?.kind.oneofKind).toBe("col");
  });

  it("Attr.cohortLive emits a cohort_live arm with optional bucketKey", () => {
    const e = Attr.cohortLive("hot");
    if (e.kind.oneofKind !== "cohortLive") throw new Error("not a cohortLive");
    expect(e.kind.cohortLive.name).toBe("hot");
    expect(e.kind.cohortLive.bucketKey).toBeUndefined();

    const e2 = Attr.cohortLive("hot", Expr.col("bucket"));
    if (e2.kind.oneofKind !== "cohortLive") throw new Error("narrow");
    expect(e2.kind.cohortLive.bucketKey?.kind.oneofKind).toBe("col");
  });

  it("Attr.cohortDraw rejects empty name or missing slot", () => {
    expect(() => Attr.cohortDraw("", Expr.lit(0))).toThrow();
    expect(() =>
      // undefined slot — mirrors a workload author forgetting the arg.
      Attr.cohortDraw("hot", undefined as unknown as ReturnType<typeof Expr.lit>),
    ).toThrow();
  });
});

describe("Expr.choose", () => {
  it("emits Choose with stream_id=0 and parallel weight/expr", () => {
    const e = Expr.choose([
      { weight: 1, expr: Expr.lit("critical") },
      { weight: 9, expr: Expr.lit("normal") },
    ]);
    if (e.kind.oneofKind !== "choose") throw new Error("not a choose");
    expect(e.kind.choose.streamId).toBe(0);
    expect(e.kind.choose.branches).toHaveLength(2);
    expect(e.kind.choose.branches[0].weight).toBe("1");
    expect(e.kind.choose.branches[1].weight).toBe("9");
  });

  it("rejects empty branches and non-positive weights", () => {
    expect(() => Expr.choose([])).toThrow();
    expect(() =>
      Expr.choose([{ weight: 0, expr: Expr.lit("x") }]),
    ).toThrow();
  });
});

describe("Dict dedup: cohort entity-range and joint draws", () => {
  it("same dict inline in two attrs (via Draw.dict) lands as one entry", () => {
    const d1 = Dict.values(["A", "B", "C"]);
    const d2 = Dict.values(["A", "B", "C"]);
    const spec = Rel.table("t", {
      size: 10,
      attrs: {
        col1: Draw.dict(d1),
        col2: Draw.dict(d2),
      },
    });
    const keys = Object.keys(spec.dicts);
    expect(keys).toHaveLength(1);
    const key = keys[0];

    const first = spec.source!.attrs[0].expr!;
    if (first.kind.oneofKind !== "streamDraw") throw new Error("expected streamDraw");
    const arm = first.kind.streamDraw.draw;
    if (arm.oneofKind !== "dict") throw new Error("expected dict arm");
    expect(arm.dict.dictKey).toBe(key);
  });

  it("Draw.phrase vocab dict shows up in spec.dicts", () => {
    const vocab = Dict.values(["alpha", "beta", "gamma"]);
    const spec = Rel.table("t", {
      size: 3,
      attrs: {
        phrase: Draw.phrase({
          vocab,
          minWords: Expr.lit(1),
          maxWords: Expr.lit(2),
        }),
      },
    });
    expect(Object.keys(spec.dicts)).toHaveLength(1);
  });
});

describe("Rel.table with cohorts", () => {
  it("threads Rel.cohort into RelSource.cohorts", () => {
    const c = Rel.cohort({
      name: "hot",
      cohortSize: 20,
      entityMin: 1,
      entityMax: 500,
      activeEvery: 3,
    });
    const spec = Rel.table("events", {
      size: 100,
      attrs: {
        row_index: Attr.rowIndex(),
        item: Attr.cohortDraw("hot", Expr.lit(0), Expr.col("row_index")),
        alive: Attr.cohortLive("hot", Expr.col("row_index")),
      },
      cohorts: [c],
    });
    expect(spec.source?.cohorts).toHaveLength(1);
    expect(spec.source?.cohorts[0].name).toBe("hot");
    expect(spec.source?.cohorts[0].cohortSize).toBe("20");
  });
});
