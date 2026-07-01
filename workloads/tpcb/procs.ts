// TPC-B, stored-procedure variant: each iteration issues a single server-side
// `tpcb_transaction` call. Shares all load/prepare/config logic with tx.ts via
// tpcb_common.ts; only the transaction body differs. pg + mysql only —
// picodata and ydb have no stored procedures (use tx.ts there).
import { Step } from "./helpers.ts";
import {
  driver,
  sql,
  driverType,
  options as scenarioOptions,
  prepare,
  teardown,
  aidGen,
  tidGen,
  bidGen,
  deltaGen,
  nextHid,
} from "./tpcb_common.ts";

// options re-declared (not `export { … }`) so the catalog's entrypoint scan finds it.
export const options = scenarioOptions;
export { teardown };

if (driverType === "picodata" || driverType === "ydb") {
  throw new Error(
    `tpcb/procs.ts only supports postgres and mysql (got driverType=${driverType}). ` +
    `Use tpcb/tx.ts for picodata/ydb.`,
  );
}

export default function (): void {
  // Load runs once across all VUs (process-global); the measured workload is a
  // separate, skippable step so prep and measure can run as two passes.
  prepare(true);

  Step("workload", () => {
    driver.exec(sql("workload_procs", "tpcb_transaction")!, {
      p_aid: aidGen.next(),
      p_tid: tidGen.next(),
      p_bid: bidGen.next(),
      p_delta: deltaGen.next(),
      p_hid: nextHid(),
    });
  }, { silent: true });
}
