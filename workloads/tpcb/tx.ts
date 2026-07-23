// TPC-B, client-side transaction variant: each iteration runs pgbench's
// canonical 5-statement script under one explicit transaction. The SELECT is a
// real round-trip — abalance is pulled back via tx.queryValue so the read
// materializes client-side (that is what pgbench measures). Shares all
// load/prepare/config logic with procs.ts via tpcb_common.ts; supports all four
// drivers (pg/mysql/picodata/ydb).
import { Step, retryWithPolicy, txRetryPolicy } from "./helpers.ts";
import {
  driver,
  sql,
  options as scenarioOptions,
  prepare,
  teardown,
  TX_ISOLATION,
  aidGen,
  tidGen,
  bidGen,
  deltaGen,
  nextHid,
  driverType,
  RETRY_ATTEMPTS,
  tpcbRetryAttempts,
} from "./tpcb_common.ts";

// options re-declared (not `export { … }`) so the catalog's entrypoint scan finds it.
export const options = scenarioOptions;
export { teardown };

// Retry serialization/transient failures (PG SQLSTATE 40001/40P01, MySQL 1213,
// YDB operation/ABORTED + "Transaction locks invalidated") the same way tpcc
// does: at SF=1 every VU contends on the single pgbench_branches row, so
// serializable aborts are expected and must be replayed, not thrown.
const tpcbTxRetryPolicy = txRetryPolicy(driverType, {
  maxAttempts: RETRY_ATTEMPTS,
  onRetry: () => { tpcbRetryAttempts.add(1); },
});
function tpcbRetry<T>(fn: () => T): T {
  return retryWithPolicy(tpcbTxRetryPolicy, fn);
}

export default function (): void {
  // Load runs once across all VUs (process-global); the measured workload is a
  // separate, skippable step so prep and measure can run as two passes.
  prepare(false);

  const aid = aidGen.next();
  const tid = tidGen.next();
  const bid = bidGen.next();
  const delta = deltaGen.next();
  const hid = nextHid();

  Step("workload", () => {
    tpcbRetry(() => driver.beginTx({ isolation: TX_ISOLATION, name: "tpcb" }, (tx) => {
      tx.exec(sql("workload_tx_tpcb", "update_account")!, { aid, delta });

      const abalance = tx.queryValue<number>(
        sql("workload_tx_tpcb", "get_balance")!, { aid },
      );
      if (abalance === undefined) {
        throw new Error(`TPC-B: account ${aid} not found`);
      }

      tx.exec(sql("workload_tx_tpcb", "update_teller")!, { tid, delta });
      tx.exec(sql("workload_tx_tpcb", "update_branch")!, { bid, delta });
      tx.exec(sql("workload_tx_tpcb", "insert_history")!, { hid, tid, bid, aid, delta });
    }));
  }, { silent: true });
}
