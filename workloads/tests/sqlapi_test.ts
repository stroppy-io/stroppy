import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";

import { DriverX, declareDriverSetup } from "./helpers.ts";

export const options: Options = {
  iterations: 1,
  vus: 1,
};

const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
});

const driver = DriverX.create().setup(driverConfig);

function assert(condition: boolean, msg: string) {
  if (!condition) throw new Error(`ASSERT FAILED: ${msg}`);
}

export default function () {
  // -- setup: create temp table --
  driver.exec("DROP TABLE IF EXISTS _sqlapi_test");
  driver.exec(
    "CREATE TABLE _sqlapi_test (id serial PRIMARY KEY, name text, value int)",
  );

  // -- exec: insert rows --
  driver.exec("INSERT INTO _sqlapi_test (name, value) VALUES ('alice', 10)");
  driver.exec("INSERT INTO _sqlapi_test (name, value) VALUES ('bob', 20)");
  driver.exec("INSERT INTO _sqlapi_test (name, value) VALUES ('carol', 30)");
  console.log("exec: inserts OK");

  // -- exec: returns stats with elapsed --
  const stats = driver.exec("SELECT 1");
  assert(
    stats.elapsed.milliseconds() >= 0,
    "exec should return stats with elapsed",
  );
  console.log(`exec: stats.elapsed = ${stats.elapsed.milliseconds()}ms`);

  // -- queryValue: single scalar --
  const count = driver.queryValue<number>(
    "SELECT count(*) FROM _sqlapi_test",
  );
  assert(count === 3, `queryValue: expected 3, got ${count}`);
  console.log(`queryValue: count = ${count}`);

  // -- queryValue: returns undefined for empty result --
  const empty = driver.queryValue(
    "SELECT id FROM _sqlapi_test WHERE id = -1",
  );
  assert(empty === undefined, `queryValue: expected undefined, got ${empty}`);
  console.log("queryValue: empty = undefined OK");

  // -- queryRow: single row, destructurable --
  const row = driver.queryRow(
    "SELECT name, value FROM _sqlapi_test ORDER BY id LIMIT 1",
  );
  assert(row !== undefined, "queryRow: should return a row");
  const [name, value] = row!;
  assert(name === "alice", `queryRow: expected alice, got ${name}`);
  assert(value === 10, `queryRow: expected 10, got ${value}`);
  console.log(`queryRow: [${name}, ${value}] OK`);

  // -- queryRow: returns undefined for empty result --
  const emptyRow = driver.queryRow(
    "SELECT * FROM _sqlapi_test WHERE id = -1",
  );
  assert(
    emptyRow === undefined,
    `queryRow: expected undefined, got ${emptyRow}`,
  );
  console.log("queryRow: empty = undefined OK");

  // -- queryRows: all rows --
  const allRows = driver.queryRows(
    "SELECT name, value FROM _sqlapi_test ORDER BY id",
  );
  assert(allRows.length === 3, `queryRows: expected 3 rows, got ${allRows.length}`);
  assert(allRows[0][0] === "alice", `queryRows[0]: expected alice`);
  assert(allRows[1][0] === "bob", `queryRows[1]: expected bob`);
  assert(allRows[2][0] === "carol", `queryRows[2]: expected carol`);
  console.log(`queryRows: ${allRows.length} rows OK`);

  // -- queryRows: with limit --
  const limited = driver.queryRows(
    "SELECT name FROM _sqlapi_test ORDER BY id",
    {},
    2,
  );
  assert(limited.length === 2, `queryRows(limit=2): expected 2, got ${limited.length}`);
  console.log(`queryRows(limit=2): ${limited.length} rows OK`);

  // -- queryCursor: manual iteration --
  const result = driver.queryCursor(
    "SELECT value FROM _sqlapi_test ORDER BY id",
  );
  const values: number[] = [];
  while (result.rows.next()) {
    values.push(result.rows.values()[0] as number);
  }
  assert(values.length === 3, `queryCursor: expected 3 values`);
  assert(values[0] === 10 && values[1] === 20 && values[2] === 30,
    `queryCursor: expected [10,20,30], got [${values}]`);
  console.log(`queryCursor: [${values}] OK`);

  // -- tags: TaggedQuery syntax --
  const taggedStats = driver.exec({
    sql: "SELECT 1",
    tags: { op: "health_check" },
  });
  assert(taggedStats.elapsed.milliseconds() >= 0, "tagged exec should work");
  console.log("TaggedQuery: OK");

  // -- cleanup --
  driver.exec("DROP TABLE _sqlapi_test");
  console.log("cleanup: OK");

  console.log("--- ALL TESTS PASSED ---");
}

export function teardown() {
  Teardown();
}
