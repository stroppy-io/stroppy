import { describe, it, expect } from "vitest";
import {
  Attr,
  Deg,
  Dict,
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
