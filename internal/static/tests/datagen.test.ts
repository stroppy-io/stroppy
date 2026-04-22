import { describe, it, expect } from "vitest";
import {
  Attr,
  Dict,
  Expr,
  Rel,
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
