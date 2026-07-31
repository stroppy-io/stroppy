package bench

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsSerializationError reports whether err is a retryable serialization/deadlock
// failure. The rollback sentinel injected by TPC-C ("tpcc_rollback:") is never
// retryable. Driver errors are matched by typed errors.As on the underlying
// pgconn/mysql error plus its SQLSTATE, replacing the TS regex port. YDB's
// transient tx errors have no SQLSTATE and stay text-based (see TxRetryPolicy).
func IsSerializationError(err error) bool {
	if err == nil {
		return false
	}

	if strings.Contains(err.Error(), "tpcc_rollback:") {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01": // serialization_failure, deadlock_detected
			return true
		}
	}

	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		switch myErr.Number {
		case 1213, 1205: // ER_LOCK_DEADLOCK, ER_LOCK_WAIT_TIMEOUT
			return true
		}
	}

	return false
}

// Retry runs fn up to maxAttempts times, retrying while isRetryable returns true.
// No backoff: serialization retries are immediate by design. onRetry fires once
// per retry, before re-invoking fn, with the upcoming (2-based) attempt number.
func Retry[T any](
	maxAttempts int,
	isRetryable func(error) bool,
	fn func() (T, error),
	onRetry ...func(int, error),
) (T, error) {
	var (
		lastErr error
		zero    T
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		v, err := fn()
		if err == nil {
			return v, nil
		}

		lastErr = err
		if !isRetryable(err) || attempt == maxAttempts {
			return zero, err
		}

		for _, cb := range onRetry {
			cb(attempt+1, err)
		}
	}

	return zero, lastErr // unreachable — last iteration always returns
}

// Retry0 is RetryWithPolicy for a void fn (the common tx-replay shape).
func Retry0(policy RetryPolicy, fn func() error) error {
	_, err := RetryWithPolicy(policy, func() (struct{}, error) {
		return struct{}{}, fn()
	})

	return err
}

// RetryDecision is the verdict a RetryPolicy.Classify returns for one error.
type RetryDecision struct {
	Retry        bool
	DelaySeconds float64
	Reason       string
}

// RetryPolicy owns error classification and optional backoff for RetryWithPolicy.
type RetryPolicy struct {
	MaxAttempts int
	Classify    func(err error, attempt int) RetryDecision
	OnRetry     func(attempt int, err error, decision RetryDecision)
}

// RetryWithPolicy runs fn under policy. The policy owns classification and
// optional backoff; the caller owns the transaction closure being replayed.
func RetryWithPolicy[T any](policy RetryPolicy, fn func() (T, error)) (T, error) {
	var (
		lastErr error
		zero    T
	)

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		v, err := fn()
		if err == nil {
			return v, nil
		}

		lastErr = err

		decision := policy.Classify(err, attempt)
		if !decision.Retry || attempt == policy.MaxAttempts {
			return zero, err
		}

		if policy.OnRetry != nil {
			policy.OnRetry(attempt+1, err, decision)
		}

		if decision.DelaySeconds > 0 {
			sleepForRetry(decision.DelaySeconds)
		}
	}

	return zero, lastErr // unreachable
}

// TxRetryPolicyOptions configures TxRetryPolicy.
type TxRetryPolicyOptions struct {
	MaxAttempts      int
	BaseDelaySeconds float64
	MaxDelaySeconds  float64
	OnRetry          func(attempt int, err error, decision RetryDecision)
}

// TxRetryPolicy builds the standard transaction-retry policy: immediate retry on
// serialization errors, exponential backoff on YDB transient errors, never on the
// rollback sentinel. driverType selects the YDB backoff branch.
func TxRetryPolicy(driverType DriverTypeName, opts TxRetryPolicyOptions) RetryPolicy {
	if opts.BaseDelaySeconds == 0 {
		opts.BaseDelaySeconds = 0.05
	}

	if opts.MaxDelaySeconds == 0 {
		opts.MaxDelaySeconds = 1
	}

	return RetryPolicy{
		MaxAttempts: opts.MaxAttempts,
		OnRetry:     opts.OnRetry,
		Classify: func(err error, attempt int) RetryDecision {
			if err == nil || strings.Contains(err.Error(), "tpcc_rollback:") {
				return RetryDecision{}
			}

			if IsSerializationError(err) {
				return RetryDecision{Retry: true, Reason: "serialization"}
			}

			if driverType == DriverYDB && isYDBTransientTxError(err.Error()) {
				return RetryDecision{
					Retry: true, Reason: "ydb_transient",
					DelaySeconds: backoffSeconds(attempt, opts.BaseDelaySeconds, opts.MaxDelaySeconds),
				}
			}

			return RetryDecision{}
		},
	}
}

// backoffSeconds is exponential with a +-20% jitter, capped at maxSeconds.
func backoffSeconds(attempt int, base, max float64) float64 {
	retryIndex := max0(attempt - 1)
	capped := min(max, base*pow2(retryIndex))

	return capped + rand.Float64()*capped*0.2
}

func sleepForRetry(seconds float64) {
	timer := time.NewTimer(time.Duration(seconds * float64(time.Second)))
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-context.Background().Done():
	}
}

// isYDBTransientTxError matches gRPC status/issue text from ydb-go-sdk, including
// split/offline shard states that never surface as a SQLSTATE.
func isYDBTransientTxError(msg string) bool {
	for _, pat := range ydbTransientPatterns {
		if strings.Contains(strings.ToLower(msg), pat) {
			return true
		}
	}

	return false
}

var ydbTransientPatterns = []string{
	"operation/overloaded",
	"operation/aborted",
	"operation/unavailable",
	"operation/bad_session",
	"operation/session_busy",
	"code = 400050",
	"code = 400060",
	"code = 400100",
	"wrong_shard_state",
	"transaction locks invalidated",
}

func max0(n int) int {
	if n < 0 {
		return 0
	}

	return n
}

func pow2(n int) float64 {
	r := 1.0
	for range n {
		r *= 2
	}

	return r
}
