import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";

import { DriverX, declareDriverSetup } from "./helpers.ts";
import { DrawRT, Alphabet, Dict, SampleableDraw } from "./datagen.ts";

// Showcase + API-contract test for the DrawRT tx-time surface.
// Exercises every DrawRT arm and asserts the three invariants each
// sobek-bound Drawer must satisfy:
//   1. determinism   — two fresh instances with identical args produce
//                      identical sequences under .next().
//   2. seekability   — .seek(k).next() === .sample(seed, k) for the
//                      struct's own seed.
//   3. reset         — after N .next() calls, .reset() returns the
//                      cursor to key=0 so the next .next() equals the
//                      first-ever emitted value.
// Runs against driverType=noop so it can execute in environments
// without a live database.
//
// Every SampleableDraw is built at k6 INIT scope (module top-level),
// not inside default(). k6's require() is only available during init,
// so DrawRT.* constructors — which lazy-load the xk6 stroppy module
// on first call — must fire before the VU runtime starts.

export const options: Options = {
  iterations: 1,
  vus: 1,
};

const driverConfig = declareDriverSetup(0, { driverType: "noop" });
const driver = DriverX.create().setup(driverConfig);

const SEED = 0x12345678;

// Dicts for DrawRT.dict / phrase / grammar showcases. Each Dict.values
// call produces an inline PbDict keyed by its content hash; DrawRT.*
// pulls the registered body on first use.
const colors = Dict.values(["red", "green", "blue", "violet"]);
const vocab = Dict.values(["alpha", "beta", "gamma", "delta", "epsilon"]);

// Minimal grammar: root dict holds the single letter "L", which
// expands directly to the grLeaf dict.
const grRoot = Dict.values(["L"]);
const grLeaf = Dict.values(["foo", "bar", "baz"]);

// Each arm needs several fresh Drawer instances (one for determinism
// comparison, two more for seek(K) at K=0/K=3, one for reset). Build
// them at init scope because DrawRT constructors call require() which
// is only legal in the k6 init stage.
interface ArmFixture {
  name: string;
  a: SampleableDraw;
  b: SampleableDraw;
  seek0: SampleableDraw;
  seekSample0: SampleableDraw;
  seek3: SampleableDraw;
  seekSample3: SampleableDraw;
  reset: SampleableDraw;
}

function fixture(name: string, make: () => SampleableDraw): ArmFixture {
  return {
    name,
    a: make(),
    b: make(),
    seek0: make(),
    seekSample0: make(),
    seek3: make(),
    seekSample3: make(),
    reset: make(),
  };
}

const arms: ArmFixture[] = [
  fixture("intUniform", () => DrawRT.intUniform(SEED, 1, 100)),
  fixture("floatUniform", () => DrawRT.floatUniform(SEED, 0, 1)),
  fixture("normal", () => DrawRT.normal(SEED, 0, 100, { screw: 1 })),
  fixture("zipf", () => DrawRT.zipf(SEED, 1, 1000, { exponent: 1.2 })),
  fixture("nurand", () => DrawRT.nurand(SEED, 255, 0, 999)),
  fixture("bernoulli", () => DrawRT.bernoulli(SEED, 0.3)),
  fixture("date", () => DrawRT.date(SEED, new Date("2020-01-01"), new Date("2024-12-31"))),
  fixture("decimal", () => DrawRT.decimal(SEED, 0, 1000, { scale: 2 })),
  fixture("ascii", () => DrawRT.ascii(SEED, 8, 12, Alphabet.en)),
  fixture("dict", () => DrawRT.dict(SEED, colors)),
  fixture("phrase", () => DrawRT.phrase(SEED, vocab, 2, 4)),
  fixture("grammar", () => DrawRT.grammar(SEED, 64, { rootDict: grRoot, leaves: { L: grLeaf } })),
];

function assert(condition: boolean, msg: string): void {
  if (!condition) throw new Error(`ASSERT FAILED: ${msg}`);
}

function eq(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

// assertArmInvariants drives the three-way contract on an ArmFixture.
function assertArmInvariants(f: ArmFixture): void {
  const N = 5;

  // 1. Determinism: two independent instances sharing the same seed
  //    + args produce identical .next() sequences.
  const seqA: unknown[] = [];
  const seqB: unknown[] = [];
  for (let i = 0; i < N; i++) seqA.push(f.a.next());
  for (let i = 0; i < N; i++) seqB.push(f.b.next());
  assert(
    eq(seqA, seqB),
    `${f.name}: determinism — A=${JSON.stringify(seqA)} B=${JSON.stringify(seqB)}`,
  );

  // 2. Seekability: seek(K).next() matches sample(SEED, K) at the
  //    same key. K=0 and K=3 cover both the cursor's initial state
  //    and a post-seek state.
  f.seek0.seek(0);
  const next0 = f.seek0.next();
  const sample0 = f.seekSample0.sample(SEED, 0);
  assert(
    eq(next0, sample0),
    `${f.name}: seek(0).next() != sample(SEED,0) — next=${JSON.stringify(next0)} sample=${JSON.stringify(sample0)}`,
  );

  f.seek3.seek(3);
  const next3 = f.seek3.next();
  const sample3 = f.seekSample3.sample(SEED, 3);
  assert(
    eq(next3, sample3),
    `${f.name}: seek(3).next() != sample(SEED,3) — next=${JSON.stringify(next3)} sample=${JSON.stringify(sample3)}`,
  );

  // 3. Reset: after draining N values, reset() restores the cursor
  //    so the next draw equals the very first seqA value.
  for (let i = 0; i < N; i++) f.reset.next();
  f.reset.reset();
  const firstAfterReset = f.reset.next();
  assert(
    eq(firstAfterReset, seqA[0]),
    `${f.name}: reset — expected ${JSON.stringify(seqA[0])}, got ${JSON.stringify(firstAfterReset)}`,
  );

  console.log(`${f.name} first-${N}: ${JSON.stringify(seqA)}`);
}

export default function (): void {
  for (const f of arms) assertArmInvariants(f);
  console.log("--- ALL DrawRT API invariants hold ---");
  // Prove the driver stood up under noop so the broader test harness
  // is exercised, not just the init-scope generator construction.
  driver.exec("SELECT 1");
}

export function teardown(): void {
  Teardown();
}
