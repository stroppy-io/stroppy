package bench

import (
	"context"
	"errors"
	"maps"
	"math/rand/v2"
	"time"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

// ErrorAction is workload policy for one classified driver error.
type ErrorAction uint8

const (
	ErrorActionError ErrorAction = iota
	ErrorActionRetry
	ErrorActionIgnore
	ErrorActionFatal
)

// ErrorActionMap maps database-independent error kinds to workload actions.
type ErrorActionMap map[driver.ErrorKind]ErrorAction

// DefaultErrorActions returns safe transaction defaults. Contention and
// unconditional transient failures retry; everything else remains an error.
func DefaultErrorActions(idempotent bool) ErrorActionMap {
	actions := ErrorActionMap{
		driver.ErrorKindSerialization: ErrorActionRetry,
		driver.ErrorKindDeadlock:      ErrorActionRetry,
		driver.ErrorKindLockTimeout:   ErrorActionRetry,
		driver.ErrorKindTransient:     ErrorActionRetry,
	}
	if idempotent {
		actions[driver.ErrorKindTransientIfIdempotent] = ErrorActionRetry
	}

	return actions
}

// With returns a copy with overrides applied. Missing kinds keep the safe
// ErrorActionError fallback.
func (m ErrorActionMap) With(overrides ErrorActionMap) ErrorActionMap {
	result := make(ErrorActionMap, len(m)+len(overrides))
	maps.Copy(result, m)
	maps.Copy(result, overrides)

	return result
}

func (m ErrorActionMap) action(kind driver.ErrorKind) ErrorAction {
	if action, ok := m[kind]; ok {
		return action
	}

	return ErrorActionError
}

// FatalError marks an error that must stop the whole scenario.
type FatalError struct {
	err error
}

func (e *FatalError) Error() string { return e.err.Error() }
func (e *FatalError) Unwrap() error { return e.err }

// IsFatalError reports whether err carries a fatal workload action.
func IsFatalError(err error) bool {
	_, ok := errors.AsType[*FatalError](err)

	return ok
}

// Retry0 is RetryWithPolicy for a void fn (the common transaction-replay shape).
func Retry0(ctx context.Context, policy RetryPolicy, fn func() error) error {
	_, err := RetryWithPolicy(ctx, policy, func() (struct{}, error) {
		return struct{}{}, fn()
	})

	return err
}

// RetryDecision records the classified facts and resolved action for one error.
type RetryDecision struct {
	Action       ErrorAction
	Facts        driver.ErrorFacts
	DelaySeconds float64
}

// RetryPolicy combines driver classification with workload-owned actions.
type RetryPolicy struct {
	MaxAttempts int
	Classify    func(error) driver.ErrorFacts
	Actions     ErrorActionMap
	OnRetry     func(attempt int, err error, decision RetryDecision)

	baseDelaySeconds float64
	maxDelaySeconds  float64
}

// RetryWithPolicy runs fn under policy. Error and fatal actions preserve the
// original error, ignore returns the zero value, and retry replays fn.
func RetryWithPolicy[T any](
	ctx context.Context,
	policy RetryPolicy,
	fn func() (T, error),
) (T, error) {
	var (
		lastErr error
		zero    T
	)

	maxAttempts := max(policy.MaxAttempts, 1)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}

		value, err := fn()
		if err == nil {
			return value, nil
		}
		lastErr = err

		facts := driver.DefaultErrorFacts(err)
		if policy.Classify != nil {
			facts = policy.Classify(err)
		}

		decision := RetryDecision{
			Action: policy.Actions.action(facts.Kind),
			Facts:  facts,
		}

		switch decision.Action {
		case ErrorActionIgnore:
			return zero, nil
		case ErrorActionFatal:
			return zero, &FatalError{err: err}
		case ErrorActionRetry:
			if attempt == maxAttempts {
				return zero, err
			}

			if facts.Backoff {
				decision.DelaySeconds = backoffSeconds(
					attempt,
					policy.baseDelaySeconds,
					policy.maxDelaySeconds,
				)
			}

			if policy.OnRetry != nil {
				policy.OnRetry(attempt+1, err, decision)
			}

			if err := sleepForRetry(ctx, decision.DelaySeconds); err != nil {
				return zero, err
			}
		default:
			return zero, err
		}
	}

	return zero, lastErr // unreachable: final attempt returns above
}

// TxRetryPolicyOptions configures Bench.TxRetryPolicy.
type TxRetryPolicyOptions struct {
	MaxAttempts      int
	Idempotent       bool
	Actions          ErrorActionMap
	BaseDelaySeconds float64
	MaxDelaySeconds  float64
	OnRetry          func(attempt int, err error, decision RetryDecision)
}

// TxRetryPolicy builds a policy from this bench's driver classifier and
// workload overrides.
func (b *Bench) TxRetryPolicy(opts TxRetryPolicyOptions) RetryPolicy {
	return newTxRetryPolicy(b.drv.ClassifyError, opts)
}

func newTxRetryPolicy(
	classify func(error) driver.ErrorFacts,
	opts TxRetryPolicyOptions,
) RetryPolicy {
	if opts.BaseDelaySeconds == 0 {
		opts.BaseDelaySeconds = 0.05
	}
	if opts.MaxDelaySeconds == 0 {
		opts.MaxDelaySeconds = 1
	}

	return RetryPolicy{
		MaxAttempts:      opts.MaxAttempts,
		Classify:         classify,
		Actions:          DefaultErrorActions(opts.Idempotent).With(opts.Actions),
		OnRetry:          opts.OnRetry,
		baseDelaySeconds: opts.BaseDelaySeconds,
		maxDelaySeconds:  opts.MaxDelaySeconds,
	}
}

// backoffSeconds is exponential with a +-20% jitter, capped at maxSeconds.
func backoffSeconds(attempt int, base, maxSeconds float64) float64 {
	retryIndex := max0(attempt - 1)
	capped := min(maxSeconds, base*pow2(retryIndex))

	return capped + rand.Float64()*capped*0.2 //nolint:gosec // G404: retry backoff jitter RNG, not security-sensitive
}

func sleepForRetry(ctx context.Context, seconds float64) error {
	if seconds <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(time.Duration(seconds * float64(time.Second)))
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
