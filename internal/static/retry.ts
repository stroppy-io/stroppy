// ============================================================================
// T2.3: SQLSTATE 40001 / deadlock retry helper
// ============================================================================
//
// PG REPEATABLE READ uses snapshot isolation, so concurrent updates to the
// same row abort with SQLSTATE 40001 ("could not serialize access due to
// concurrent update"). MySQL's row-locking equivalent is "Deadlock found
// when trying to get lock" (Error 1213, SQLSTATE 40001). The TPC-C spec
// §5.2.5 caps the total error rate at 1%, so we retry these aborts a few
// times before letting them bubble up to k6's tx_error_rate.
//
// Audit (2026-04-09) of pkg/driver/{postgres,mysql,sqldriver,...}: the
// stroppy Go layer wraps every driver error with `fmt.Errorf("...: %w",
// err)` (see sqldriver/run_query.go and cmd/xk6air/driver_wrapper.go). Both
// pgx's pgconn.PgError.Error() and go-sql-driver's mysql.MySQLError.Error()
// stringify to formats that include enough textual signal:
//
//   pg:    "ERROR: could not serialize access due to concurrent update (SQLSTATE 40001)"
//   pg:    "ERROR: deadlock detected (SQLSTATE 40P01)"
//   mysql: "Error 1213 (40001): Deadlock found when trying to get lock; ..."
//
// So no Go-side classification is needed — the JS catch sees the wrapped
// message via `e.message` and a regex match is enough.
//
// IMPORTANT: this MUST return false for the spec §2.4.2.3 New-Order rollback
// sentinel ("tpcc_rollback:item_not_found"). On PG that path is raised via
// `RAISE EXCEPTION` (SQLSTATE P0001) and on MySQL via `SIGNAL SQLSTATE
// '45000'`, neither of which match the serialization patterns below — but we
// add an explicit early-out so a future regex tweak can't accidentally trip
// the rollback path.
//
// YDB: the ydb-go-sdk surfaces gRPC status text (operation/UNAVAILABLE,
// operation/ABORTED, wrong shard state during tablet splits, etc.) rather
// than SQLSTATE. txRetryPolicy() matches those via isYDBTransientTxErrorMessage
// and applies exponential backoff (default 50 ms → 1 s) before replaying the
// transaction closure. Serialization-class errors still retry immediately.
//
// Re-exported from helpers.ts for workload imports.

export type TxRetryDriverType = "postgres" | "mysql" | "picodata" | "ydb" | "noop" | "csv";

/** Check if an error is a transient serialization failure or deadlock that
 *  should be retried. Matches both PostgreSQL and MySQL error texts.
 *  Explicitly excludes the TPC-C `tpcc_rollback:` sentinel so the spec
 *  §2.4.2.3 New-Order rollback path is never retried. */
/* eslint-disable @typescript-eslint/no-explicit-any */
export const isSerializationError = (e: any): boolean => {
  const msg = String(e?.message ?? e);
  // Defensive: never retry the spec-mandated rollback sentinel.
  if (msg.indexOf("tpcc_rollback:") >= 0) return false;
  return /SQLSTATE 40001/i.test(msg)
      || /could not serialize access/i.test(msg)
      || /SQLSTATE 40P01/i.test(msg)
      || /deadlock detected/i.test(msg)        // pg
      || /Deadlock found/i.test(msg)           // mysql
      || /Error 1213/i.test(msg);              // mysql numeric
};
/* eslint-enable @typescript-eslint/no-explicit-any */

/** Run `fn`, retrying up to `maxAttempts - 1` times when `isRetryable(e)`
 *  returns true. The optional `onRetry` callback fires once per retry,
 *  before re-invoking `fn`, and receives the upcoming attempt number
 *  (2-based) plus the caught error. No backoff: serialization retries are
 *  immediate by design — sleeping inside a tx body would just deepen the
 *  contention window. Use the callback to bump a counter so operators can
 *  observe how often retries fire. */
/* eslint-disable @typescript-eslint/no-explicit-any */
export const retry = <T>(
  maxAttempts: number,
  isRetryable: (e: any) => boolean,
  fn: () => T,
  onRetry?: (attempt: number, e: any) => void,
): T => {
  let lastErr: any;
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      return fn();
    } catch (e) {
      lastErr = e;
      if (!isRetryable(e) || attempt === maxAttempts) {
        throw e;
      }
      if (onRetry) onRetry(attempt + 1, e);
    }
  }
  throw lastErr; // unreachable — last iteration always throws or returns
};
/* eslint-enable @typescript-eslint/no-explicit-any */

export interface RetryDecision {
  retry: boolean;
  delaySeconds?: number;
  reason?: string;
}

/* eslint-disable @typescript-eslint/no-explicit-any */
export interface RetryPolicy {
  maxAttempts: number;
  classify: (e: any, attempt: number) => boolean | RetryDecision;
  onRetry?: (attempt: number, e: any, decision: RetryDecision) => void;
}
/* eslint-enable @typescript-eslint/no-explicit-any */

function normalizeRetryDecision(decision: boolean | RetryDecision): RetryDecision {
  return typeof decision === "boolean" ? { retry: decision } : decision;
}

function backoffSeconds(attempt: number, baseSeconds: number, maxSeconds: number): number {
  const retryIndex = Math.max(attempt - 1, 0);
  const capped = Math.min(maxSeconds, baseSeconds * Math.pow(2, retryIndex));
  return capped + Math.random() * capped * 0.2;
}

function sleepForRetry(seconds: number): void {
  const k6Sleep = (globalThis as typeof globalThis & { sleep?: (t: number) => void }).sleep;
  if (k6Sleep) k6Sleep(seconds);
}

// YDB transient tx errors: match gRPC status / issue text from ydb-go-sdk.
// Includes split/offline shard states (operation/UNAVAILABLE, code 400050,
// "wrong shard state") that are not visible as SQLSTATE in the error string.
function isYDBTransientTxErrorMessage(msg: string): boolean {
  return /operation\/OVERLOADED/i.test(msg)
      || /operation\/ABORTED/i.test(msg)
      || /operation\/UNAVAILABLE/i.test(msg)
      || /operation\/BAD_SESSION/i.test(msg)
      || /operation\/SESSION_BUSY/i.test(msg)
      || /code\s*=\s*400050/i.test(msg)
      || /code\s*=\s*400060/i.test(msg)
      || /code\s*=\s*400100/i.test(msg)
      || /wrong[\s_]shard[\s_]state/i.test(msg)
      || /Transaction locks invalidated/i.test(msg);
}

export interface TxRetryPolicyOptions {
  maxAttempts: number;
  baseDelaySeconds?: number;
  maxDelaySeconds?: number;
  onRetry?: RetryPolicy["onRetry"];
}

export function txRetryPolicy(
  driverType: TxRetryDriverType | undefined,
  options: TxRetryPolicyOptions,
): RetryPolicy {
  const baseDelaySeconds = options.baseDelaySeconds ?? 0.05;
  const maxDelaySeconds = options.maxDelaySeconds ?? 1;

  return {
    maxAttempts: options.maxAttempts,
    onRetry: options.onRetry,
    classify: (e, attempt) => {
      const msg = String(e?.message ?? e);
      if (msg.indexOf("tpcc_rollback:") >= 0) return false;

      if (isSerializationError(e)) {
        return { retry: true, reason: "serialization" };
      }

      if (driverType === "ydb" && isYDBTransientTxErrorMessage(msg)) {
        return {
          retry: true,
          delaySeconds: backoffSeconds(attempt, baseDelaySeconds, maxDelaySeconds),
          reason: "ydb_transient",
        };
      }

      return false;
    },
  };
}

/** Run `fn` under a retry policy. The policy owns error classification and
 * optional backoff; callers own the transaction closure being replayed. */
/* eslint-disable @typescript-eslint/no-explicit-any */
export const retryWithPolicy = <T>(
  policy: RetryPolicy,
  fn: () => T,
): T => {
  let lastErr: any;
  for (let attempt = 1; attempt <= policy.maxAttempts; attempt++) {
    try {
      return fn();
    } catch (e) {
      lastErr = e;
      const decision = normalizeRetryDecision(policy.classify(e, attempt));
      if (!decision.retry || attempt === policy.maxAttempts) {
        throw e;
      }
      if (policy.onRetry) policy.onRetry(attempt + 1, e, decision);
      const delaySeconds = decision.delaySeconds ?? 0;
      if (delaySeconds > 0) sleepForRetry(delaySeconds);
    }
  }
  throw lastErr; // unreachable — last iteration always throws or returns
};
/* eslint-enable @typescript-eslint/no-explicit-any */
