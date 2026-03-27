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
  defaultTxIsolation: "read_committed",
});

const driver = DriverX.create().setup(driverConfig);

function assert(condition: boolean, msg: string) {
  if (!condition) throw new Error(`ASSERT FAILED: ${msg}`);
}

export default function () {
  // -- setup: create temp table --
  driver.exec("DROP TABLE IF EXISTS _tx_test");
  driver.exec(
    "CREATE TABLE _tx_test (id serial PRIMARY KEY, name text, value int)",
  );

  // -- test 1: beginTx callback — auto commit --
  driver.beginTx((tx) => {
    tx.exec("INSERT INTO _tx_test (name, value) VALUES ('alice', 10)");
  });
  const count1 = driver.queryValue<number>("SELECT count(*) FROM _tx_test");
  assert(count1 === 1, `beginTx auto-commit: expected 1 row, got ${count1}`);
  console.log("test 1: beginTx auto-commit OK");

  // -- test 2: beginTx callback — auto rollback on throw --
  try {
    driver.beginTx((tx) => {
      tx.exec("INSERT INTO _tx_test (name, value) VALUES ('bob', 20)");
      throw new Error("intentional error");
    });
  } catch (e) {
    // expected
  }
  const count2 = driver.queryValue<number>("SELECT count(*) FROM _tx_test");
  assert(count2 === 1, `beginTx auto-rollback: expected 1 row, got ${count2}`);
  console.log("test 2: beginTx auto-rollback OK");

  // -- test 3: manual begin + commit --
  const tx3 = driver.begin();
  tx3.exec("INSERT INTO _tx_test (name, value) VALUES ('carol', 30)");
  tx3.commit();
  const count3 = driver.queryValue<number>("SELECT count(*) FROM _tx_test");
  assert(count3 === 2, `manual commit: expected 2 rows, got ${count3}`);
  console.log("test 3: manual begin + commit OK");

  // -- test 4: manual begin + rollback --
  const tx4 = driver.begin();
  tx4.exec("INSERT INTO _tx_test (name, value) VALUES ('dave', 40)");
  tx4.rollback();
  const count4 = driver.queryValue<number>("SELECT count(*) FROM _tx_test");
  assert(count4 === 2, `manual rollback: expected 2 rows, got ${count4}`);
  console.log("test 4: manual begin + rollback OK");

  // -- test 5: explicit isolation level --
  driver.beginTx({isolation: "serializable"}, (tx) => {
    tx.exec("INSERT INTO _tx_test (name, value) VALUES ('eve', 50)");
  });
  const count5 = driver.queryValue<number>("SELECT count(*) FROM _tx_test");
  assert(count5 === 3, `explicit isolation: expected 3 rows, got ${count5}`);
  console.log("test 5: explicit isolation (serializable) OK");

  // -- test 6: "none" mode — queries go through driver pool, no actual tx --
  driver.beginTx({isolation: "none"}, (tx) => {
    tx.exec("INSERT INTO _tx_test (name, value) VALUES ('frank', 60)");
    // In none mode, insert is immediately visible outside
    const vis = driver.queryValue<number>(
      "SELECT count(*) FROM _tx_test WHERE name = 'frank'",
    );
    assert(vis === 1, `none mode: insert should be visible immediately, got ${vis}`);
  });
  const count6 = driver.queryValue<number>("SELECT count(*) FROM _tx_test");
  assert(count6 === 4, `none mode: expected 4 rows, got ${count6}`);
  console.log("test 6: none mode OK");

  // -- test 7: queries within tx see own writes --
  driver.beginTx((tx) => {
    tx.exec("INSERT INTO _tx_test (name, value) VALUES ('grace', 70)");
    const inTx = tx.queryValue<number>(
      "SELECT count(*) FROM _tx_test WHERE name = 'grace'",
    );
    assert(inTx === 1, `in-tx visibility: expected 1, got ${inTx}`);
  });
  console.log("test 7: in-tx visibility OK");

  // -- test 8: defaultTxIsolation from DriverSetup --
  // The driver was setup with defaultTxIsolation: "read_committed"
  // begin() without args should use that default — just verify no error
  const tx8 = driver.begin();
  tx8.exec("SELECT 1");
  tx8.commit();
  console.log("test 8: defaultTxIsolation OK");

  // -- test 9: "conn" mode — pinned connection, no SQL transaction --
  driver.beginTx({isolation: "conn"}, (tx) => {
    tx.exec("INSERT INTO _tx_test (name, value) VALUES ('heidi', 80)");
    const inConn = tx.queryValue<number>(
      "SELECT count(*) FROM _tx_test WHERE name = 'heidi'",
    );
    assert(inConn === 1, `conn mode: expected 1, got ${inConn}`);
  });
  const count9 = driver.queryValue<number>("SELECT count(*) FROM _tx_test");
  assert(count9 === 6, `conn mode: expected 6 rows, got ${count9}`);
  console.log("test 9: conn mode OK");

  // -- test 10: TxX has QueryAPI methods --
  driver.beginTx((tx) => {
    const row = tx.queryRow("SELECT name, value FROM _tx_test ORDER BY id LIMIT 1");
    assert(row !== undefined, "tx queryRow should return a row");
    assert(row![0] === "alice", `tx queryRow: expected alice, got ${row![0]}`);

    const rows = tx.queryRows("SELECT name FROM _tx_test ORDER BY id", {}, 2);
    assert(rows.length === 2, `tx queryRows(limit=2): expected 2, got ${rows.length}`);

    const val = tx.queryValue<string>("SELECT name FROM _tx_test WHERE value = 30");
    assert(val === "carol", `tx queryValue: expected carol, got ${val}`);
  });
  console.log("test 10: TxX QueryAPI methods OK");

  // -- cleanup --
  driver.exec("DROP TABLE _tx_test");
  console.log("cleanup: OK");

  console.log("--- ALL TRANSACTION TESTS PASSED ---");
}

export function teardown() {
  Teardown();
}
