import { describe, it, expect, beforeEach } from "vitest";
import { DrawRT, __setDrawRTStroppyModule } from "../datagen.ts";

// fakeDrawX mimics the sobek-bound Go struct for a Draw arm. Its
// internals don't match the Go kernel (no seed composition, just a
// counter), but the shape matches what sobek would return — which is
// what we're testing: that DrawRT builders call into stroppy.* with
// the right positional arguments and surface the returned object.
class fakeDrawX {
  seed: number;
  cursor = 0;
  constructor(
    seed: number,
    public lo: number,
    public hi: number,
  ) {
    this.seed = seed;
  }
  // Deterministic fake: hash of (seed, key) folded into the [lo, hi]
  // range. Only asserts that Sample and Next match at the same (seed,
  // cursor) point.
  _at(seed: number, key: number): number {
    const mixed = (seed * 0x9e3779b1 + key * 2654435761) >>> 0;
    return this.lo + (mixed % (this.hi - this.lo + 1));
  }
  sample(seed: number, key: number): any {
    return this._at(seed, key);
  }
  next(): any {
    const v = this._at(this.seed, this.cursor);
    this.cursor++;
    return v;
  }
  seek(key: number): void {
    this.cursor = key;
  }
  reset(): void {
    this.cursor = 0;
  }
}

// The fake stroppy module. Each NewDrawX returns a fresh fakeDrawX;
// register* calls return monotonic handles.
const stubModule = {
  NewDrawIntUniform: (seed: number, lo: number, hi: number) => new fakeDrawX(seed, lo, hi),
  NewDrawFloatUniform: (seed: number, lo: number, hi: number) => new fakeDrawX(seed, lo, hi),
  NewDrawNormal: (seed: number, lo: number, hi: number, _screw: number) =>
    new fakeDrawX(seed, lo, hi),
  NewDrawZipf: (seed: number, lo: number, hi: number, _exp: number) =>
    new fakeDrawX(seed, lo, hi),
  NewDrawNURand: (seed: number, a: number, _x: number, _y: number, _c: number) =>
    new fakeDrawX(seed, 0, a),
  NewDrawBernoulli: (seed: number, _p: number) => new fakeDrawX(seed, 0, 1),
  NewDrawDate: (seed: number, lo: number, hi: number) => new fakeDrawX(seed, lo, hi),
  NewDrawDecimal: (seed: number, lo: number, hi: number, _scale: number) =>
    new fakeDrawX(seed, Math.floor(lo), Math.floor(hi)),
  NewDrawASCII: (seed: number, minLen: number, maxLen: number, _handle: number) =>
    new fakeDrawX(seed, minLen, maxLen),
  NewDrawDict: (seed: number, _handle: number, _w: string) => new fakeDrawX(seed, 0, 0),
  NewDrawJoint: (seed: number, _handle: number, _col: string, _w: string) =>
    new fakeDrawX(seed, 0, 0),
  NewDrawPhrase: (seed: number, _handle: number, minW: number, maxW: number, _sep: string) =>
    new fakeDrawX(seed, minW, maxW),
  NewDrawGrammar: (seed: number, _handle: number, minLen: number, maxLen: number) =>
    new fakeDrawX(seed, minLen, maxLen),
  RegisterDict: (_name: string, _bin: Uint8Array): number => 1,
  RegisterAlphabet: (_bin: Uint8Array): number => 2,
  RegisterGrammar: (_bin: Uint8Array): number => 3,
};

describe("DrawRT.intUniform", () => {
  beforeEach(() => __setDrawRTStroppyModule(stubModule));

  it("passes seed + numeric bounds to the Go constructor", () => {
    const d = DrawRT.intUniform(42, 1, 100) as any;
    expect(d).toBeInstanceOf(fakeDrawX);
    expect(d.seed).toBe(42);
    expect(d.lo).toBe(1);
    expect(d.hi).toBe(100);
  });

  it(".next() is deterministic across wrappers with the same seed", () => {
    const a = DrawRT.intUniform(777, 0, 1_000_000);
    const b = DrawRT.intUniform(777, 0, 1_000_000);
    for (let i = 0; i < 16; i++) {
      expect(a.next()).toBe(b.next());
    }
  });

  it("Seek + Next equals Sample(seed, key)", () => {
    const d = DrawRT.intUniform(9, 0, 1_000_000);
    // Capture seed from the stub (tests know it's accessible via seed).
    const seed = (d as any).seed as number;
    for (const key of [0, 1, 7, 42, 99]) {
      d.seek(key);
      const viaNext = d.next();
      const viaSample = d.sample(seed, key);
      expect(viaNext).toBe(viaSample);
    }
  });

  it(".reset() puts the cursor back to 0", () => {
    const d = DrawRT.intUniform(1, 10, 20);
    const first = d.next();
    d.next();
    d.next();
    d.reset();
    expect(d.next()).toBe(first);
  });
});

describe("DrawRT.nurand", () => {
  beforeEach(() => __setDrawRTStroppyModule(stubModule));

  it("forwards bigint-ish ints as numbers to the Go constructor", () => {
    const d = DrawRT.nurand(12, 255, 0, 9999) as any;
    expect(d).toBeInstanceOf(fakeDrawX);
    expect(d.seed).toBe(12);
    expect(d.lo).toBe(0);
    expect(d.hi).toBe(255);
  });

  it("honors cSalt option", () => {
    // The stub doesn't use cSalt but we ensure the call path doesn't
    // throw on the BigInt→Number coercion for salts passed as bigint.
    expect(() => DrawRT.nurand(1, 255n, 0n, 9999n, { cSalt: 0xBEEFn })).not.toThrow();
  });
});

describe("DrawRT.bernoulli", () => {
  beforeEach(() => __setDrawRTStroppyModule(stubModule));

  it("returns a SampleableDraw with the 4-method shape", () => {
    const d = DrawRT.bernoulli(5, 0.5);
    expect(typeof d.sample).toBe("function");
    expect(typeof d.next).toBe("function");
    expect(typeof d.seek).toBe("function");
    expect(typeof d.reset).toBe("function");
  });
});

describe("DrawRT coercion", () => {
  beforeEach(() => __setDrawRTStroppyModule(stubModule));

  it("rejects non-literal Expr bounds", () => {
    // Construct a non-literal Expr (RowIndex arm) and verify coercion
    // throws rather than silently passing a junk number.
    const fakeExpr: any = { kind: { oneofKind: "rowIndex", rowIndex: {} } };
    expect(() => DrawRT.intUniform(1, fakeExpr, 100)).toThrow();
  });

  it("accepts number and bigint literals directly", () => {
    expect(() => DrawRT.intUniform(1, 0, 99)).not.toThrow();
    expect(() => DrawRT.intUniform(1, 0n, 99n)).not.toThrow();
  });
});
