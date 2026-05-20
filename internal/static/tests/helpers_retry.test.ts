import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { retryWithPolicy, txRetryPolicy } from "../retry.ts";

const splitShardCommitError =
  "operation/UNAVAILABLE (code = 400050, address = rnd-ydb7.front.private:2136, nodeID = 50011, "
  + "issues = [{#2005 'Wrong shard state. Table `/rnd-ydb/database/orders/idx_order/indexImplTable`' "
  + "[{#2029 'Rejecting data TxId 281475220180053 because datashard 72075186224051389: "
  + "is in a pre/offline state assuming this is due to a finished split (wrong shard state)'}]}])";

describe("txRetryPolicy ydb transient errors", () => {
  it("classifies split/offline shard UNAVAILABLE as retryable with backoff", () => {
    const policy = txRetryPolicy("ydb", { maxAttempts: 3 });
    const decision = policy.classify({ message: splitShardCommitError }, 1);
    expect(decision).toEqual({
      retry: true,
      delaySeconds: expect.any(Number),
      reason: "ydb_transient",
    });
    expect((decision as { delaySeconds: number }).delaySeconds).toBeGreaterThan(0);
  });

  it("does not classify the same error as retryable for non-ydb drivers", () => {
    const policy = txRetryPolicy("postgres", { maxAttempts: 3 });
    expect(policy.classify({ message: splitShardCommitError }, 1)).toBe(false);
  });

  it("never retries the tpcc_rollback sentinel even on ydb", () => {
    const policy = txRetryPolicy("ydb", { maxAttempts: 3 });
    expect(policy.classify({ message: "tpcc_rollback:item_not_found" }, 1)).toBe(false);
  });
});

describe("retryWithPolicy ydb backoff", () => {
  let slept: number[];

  beforeEach(() => {
    slept = [];
    (globalThis as typeof globalThis & { sleep?: (t: number) => void }).sleep = (t: number) => {
      slept.push(t);
    };
  });

  afterEach(() => {
    delete (globalThis as typeof globalThis & { sleep?: (t: number) => void }).sleep;
  });

  it("sleeps with exponential backoff before retrying ydb transient errors", () => {
    const policy = txRetryPolicy("ydb", {
      maxAttempts: 3,
      baseDelaySeconds: 0.1,
      maxDelaySeconds: 1,
    });
    const fn = vi.fn()
      .mockImplementationOnce(() => { throw { message: splitShardCommitError }; })
      .mockImplementationOnce(() => { throw { message: splitShardCommitError }; })
      .mockReturnValue("ok");

    const result = retryWithPolicy(policy, fn);

    expect(result).toBe("ok");
    expect(fn).toHaveBeenCalledTimes(3);
    expect(slept).toHaveLength(2);
    expect(slept[0]).toBeGreaterThan(0);
    expect(slept[1]).toBeGreaterThan(slept[0]);
  });
});
