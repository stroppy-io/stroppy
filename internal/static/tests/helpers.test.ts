import { describe, expect, it, vi } from "vitest";

async function loadHelpers(env: Record<string, string> = {}) {
  vi.resetModules();
  (globalThis as unknown as { __ENV: Record<string, string> }).__ENV = env;

  return import("../helpers.ts");
}

describe("DriverSetup validation", () => {
  it("accepts valid pool overrides and deep-merges them over defaults", async () => {
    const { declareDriverSetup } = await loadHelpers({
      STROPPY_DRIVER_0: JSON.stringify({ pool: { maxConns: 20 } }),
    });

    const setup = declareDriverSetup(0, {
      driverType: "postgres",
      pool: { maxConns: 5, minConns: 5 },
    });

    expect(setup.pool).toEqual({ maxConns: 20, minConns: 5 });
  });

  it("rejects unknown pool keys transported by the CLI", async () => {
    const { declareDriverSetup } = await loadHelpers({
      STROPPY_DRIVER_0: JSON.stringify({ pool: { maximum: 20 } }),
    });

    expect(() => declareDriverSetup(0, { driverType: "postgres" })).toThrow(/unknown pool option "maximum"/);
  });

  it("rejects unknown top-level keys", async () => {
    const { declareDriverSetup } = await loadHelpers({
      STROPPY_DRIVER_0: JSON.stringify({ tls: { insecureSkipVerify: true } }),
    });

    expect(() => declareDriverSetup(0, { driverType: "postgres" })).toThrow(/unknown driver option "tls"/);
  });

  it("rejects invalid primitive types", async () => {
    const { declareDriverSetup } = await loadHelpers({
      STROPPY_DRIVER_0: JSON.stringify({ pool: { maxConns: "20" } }),
    });

    expect(() => declareDriverSetup(0, { driverType: "postgres" })).toThrow(/pool\.maxConns must be number/);
  });

  it("rejects invalid enum values", async () => {
    const { declareDriverSetup } = await loadHelpers({
      STROPPY_DRIVER_0: JSON.stringify({ driverType: "oracle" }),
    });

    expect(() => declareDriverSetup(0, { driverType: "postgres" })).toThrow(/driverType must be one of/);
  });
});
