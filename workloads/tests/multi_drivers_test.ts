import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverConfig_DriverType } from "./stroppy.pb.js";
import { DriverX, ENV, once } from "./helpers.ts";
import exec from "k6/execution";

const DRIVER_URL = ENV("DRIVER_URL", "postgres://postgres:postgres@localhost:5432", "Database connection URL");
const VUS = 3;

export const options: Options = {
  vus: VUS,
  iterations: VUS * 3, // 3 iterations per VU
};

function assert(condition: boolean, msg: string) {
  if (!condition) throw new Error(`ASSERT FAILED: ${msg}`);
}

const pgConfig = (appName: string, poolSize: number) => ({
  url: DRIVER_URL + "?application_name=" + appName,
  driverType: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
  driverSpecific: {
    oneofKind: "postgres" as const,
    postgres: { maxConns: poolSize, minConns: poolSize },
  },
});

// ---- Shared driver: created at init phase (vu.State() == nil) ----
const sharedDriver = DriverX.create().setup(pgConfig("mdt_shared", 2));

// ---- Second shared driver: proves multiple shared drivers coexist ----
const sharedDriver2 = DriverX.create().setup(pgConfig("mdt_shared2", 1));

// ---- Per-VU driver: created empty at init, setup inside default() ----
const vuDriver = DriverX.create();

// Per-VU counter: each VU has its own JS runtime, so this is VU-local.
let vuSetupLambdaCalls = 0;

export function setup() {
  sharedDriver.exec("DROP TABLE IF EXISTS _mdt_log");
  sharedDriver.exec(`CREATE TABLE _mdt_log (
    vu_id INT,
    iter INT,
    test TEXT,
    value TEXT
  )`);
}

const vuSetup = once((i: number) => { return i+1;});

export default function () {
  const vid = exec.vu.idInTest;
  const it = exec.vu.iterationInScenario;

  // Setup per-VU driver with a VU-specific application_name.
  vuDriver.setup(pgConfig("mdt_vu_" + vid, 1));
  vuSetupLambdaCalls = vuSetup(vuSetupLambdaCalls);

  // ---- Test 1: all three drivers can query ----
  assert(sharedDriver.queryValue<number>("SELECT 1") === 1, "shared driver query");
  assert(sharedDriver2.queryValue<number>("SELECT 1") === 1, "shared driver 2 query");
  assert(vuDriver.queryValue<number>("SELECT 1") === 1, "vu driver query");

  // ---- Test 2: application_name reveals pool identity ----
  const sharedApp = sharedDriver.queryValue<string>("SELECT current_setting('application_name')");
  const shared2App = sharedDriver2.queryValue<string>("SELECT current_setting('application_name')");
  const vuApp = vuDriver.queryValue<string>("SELECT current_setting('application_name')");

  // Record observations
  sharedDriver.exec(
    `INSERT INTO _mdt_log (vu_id, iter, test, value) VALUES
      (${vid}, ${it}, 'shared_app', '${sharedApp}'),
      (${vid}, ${it}, 'shared2_app', '${shared2App}'),
      (${vid}, ${it}, 'vu_app', '${vuApp}'),
      (${vid}, ${it}, 'lambda_calls', '${vuSetupLambdaCalls}')`,
  );

  console.log(
    `VU ${vid} iter ${it}: shared=${sharedApp} shared2=${shared2App} vu=${vuApp} lambda=${vuSetupLambdaCalls}`,
  );
}

export function teardown() {
  console.log("=== Verification ===");

  // 1. Shared driver: all VUs must see the same application_name
  const sharedNames = sharedDriver.queryRows(
    "SELECT DISTINCT value FROM _mdt_log WHERE test = 'shared_app'",
  );
  console.log(`shared app names: ${sharedNames.map((r) => r[0]).join(", ")}`);
  assert(
    sharedNames.length === 1 && sharedNames[0][0] === "mdt_shared",
    `expected single 'mdt_shared', got: ${sharedNames.map((r) => r[0])}`,
  );

  // 2. Second shared driver: also single name, different from the first
  const shared2Names = sharedDriver.queryRows(
    "SELECT DISTINCT value FROM _mdt_log WHERE test = 'shared2_app'",
  );
  console.log(`shared2 app names: ${shared2Names.map((r) => r[0]).join(", ")}`);
  assert(
    shared2Names.length === 1 && shared2Names[0][0] === "mdt_shared2",
    `expected single 'mdt_shared2', got: ${shared2Names.map((r) => r[0])}`,
  );

  // 3. Per-VU driver: each VU must have a distinct application_name
  const vuNames = sharedDriver.queryRows(
    "SELECT DISTINCT value FROM _mdt_log WHERE test = 'vu_app' ORDER BY value",
  );
  console.log(`vu app names: ${vuNames.map((r) => r[0]).join(", ")}`);
  assert(
    vuNames.length === VUS,
    `expected ${VUS} distinct VU app names, got ${vuNames.length}: ${vuNames.map((r) => r[0])}`,
  );
  // Each name must follow the mdt_vu_<N> pattern
  for (const row of vuNames) {
    assert(
      (row[0] as string).startsWith("mdt_vu_"),
      `unexpected VU app name: ${row[0]}`,
    );
  }

  // 4. Per-VU app name is stable across iterations of the same VU
  const vuConsistency = sharedDriver.queryRows(
    `SELECT vu_id, COUNT(DISTINCT value) AS cnt
     FROM _mdt_log WHERE test = 'vu_app'
     GROUP BY vu_id ORDER BY vu_id`,
  );
  for (const row of vuConsistency) {
    console.log(`VU ${row[0]}: distinct vu_app count = ${row[1]}`);
    assert(
      Number(row[1]) === 1,
      `VU ${row[0]} saw ${row[1]} different vu_app names (expected 1)`,
    );
  }

  // 5. Setup lambda ran exactly once per VU (not per iteration)
  const lambdaCounts = sharedDriver.queryRows(
    `SELECT vu_id, MAX(value::int) AS final_count
     FROM _mdt_log WHERE test = 'lambda_calls'
     GROUP BY vu_id ORDER BY vu_id`,
  );
  for (const row of lambdaCounts) {
    console.log(`VU ${row[0]}: lambda calls = ${row[1]}`);
    assert(
      Number(row[1]) === 1,
      `VU ${row[0]} lambda ran ${row[1]} times (expected 1)`,
    );
  }

  // 6. Verify pool separation via pg_stat_activity
  const pools = sharedDriver.queryRows(
    `SELECT application_name, COUNT(*) AS conns
     FROM pg_stat_activity
     WHERE application_name LIKE 'mdt_%'
     GROUP BY application_name
     ORDER BY application_name`,
  );
  console.log("--- Connection pools ---");
  for (const row of pools) {
    console.log(`  ${row[0]}: ${row[1]} connections`);
  }

  // shared driver pool: 2 connections
  const sharedPool = pools.find((r) => r[0] === "mdt_shared");
  assert(sharedPool !== undefined, "mdt_shared pool not found in pg_stat_activity");
  assert(
    Number(sharedPool![1]) === 2,
    `mdt_shared expected 2 connections, got ${sharedPool![1]}`,
  );

  // shared driver 2 pool: 1 connection
  const shared2Pool = pools.find((r) => r[0] === "mdt_shared2");
  assert(shared2Pool !== undefined, "mdt_shared2 pool not found in pg_stat_activity");
  assert(
    Number(shared2Pool![1]) === 1,
    `mdt_shared2 expected 1 connection, got ${shared2Pool![1]}`,
  );

  // per-VU pools: 1 connection each, one per VU
  const vuPools = pools.filter((r) => (r[0] as string).startsWith("mdt_vu_"));
  assert(
    vuPools.length === VUS,
    `expected ${VUS} VU pools, got ${vuPools.length}`,
  );
  for (const row of vuPools) {
    assert(
      Number(row[1]) === 1,
      `${row[0]} expected 1 connection, got ${row[1]}`,
    );
  }

  // Cleanup
  sharedDriver.exec("DROP TABLE IF EXISTS _mdt_log");

  console.log("--- ALL TESTS PASSED ---");
  Teardown();
}
