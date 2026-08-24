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
// transient failures retry; everything else remains an error.
func DefaultErrorActions() ErrorActionMap {
	return ErrorActionMap{ //nolint:exhaustive // absent kinds use ErrorActionError
		driver.ErrorKindSerialization: ErrorActionRetry,
		driver.ErrorKindDeadlock:      ErrorActionRetry,
		driver.ErrorKindLockTimeout:   ErrorActionRetry,
		driver.ErrorKindTransient:     ErrorActionRetry,
	}
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

	idempotent       bool
	baseDelaySeconds float64
	maxDelaySeconds  float64
}

// Retry0 runs a void operation under policy. Error and fatal actions preserve
// the original error, ignore returns nil, and retry replays fn.
func Retry0(ctx context.Context, policy RetryPolicy, fn func() error) error {
	var lastErr error

	maxAttempts := max(policy.MaxAttempts, 1)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		decision := policy.decision(err)

		done, result := applyRetryDecision(ctx, policy, attempt, maxAttempts, err, decision)

		if done {
			return result
		}
	}

	return lastErr // unreachable: final attempt returns above
}

func applyRetryDecision(
	ctx context.Context,
	policy RetryPolicy,
	attempt int,
	maxAttempts int,
	err error,
	decision RetryDecision,
) (bool, error) {
	switch decision.Action {
	case ErrorActionIgnore:
		return true, nil
	case ErrorActionFatal:
		return true, &FatalError{err: err}
	case ErrorActionRetry:
		if attempt == maxAttempts {
			return true, err
		}

		if decision.Facts.Backoff {
			decision.DelaySeconds = backoffSeconds(
				attempt,
				policy.baseDelaySeconds,
				policy.maxDelaySeconds,
			)
		}

		if policy.OnRetry != nil {
			policy.OnRetry(attempt+1, err, decision)
		}

		if decision.DelaySeconds > 0 {
			if err := sleepForRetry(ctx, decision.DelaySeconds); err != nil {
				return true, err
			}
		}

		return false, nil
	default:
		return true, err
	}
}

func (p RetryPolicy) decision(err error) RetryDecision {
	facts := classifyError(err, p.Classify)

	action := p.Actions.action(facts.Kind)
	if action == ErrorActionRetry && facts.RequiresIdempotency && !p.idempotent {
		action = ErrorActionError
	}

	return RetryDecision{Action: action, Facts: facts}
}

func classifyError(err error, classify func(error) driver.ErrorFacts) driver.ErrorFacts {
	if classify != nil {
		return classify(err)
	}

	return driver.DefaultErrorFacts(err)
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
	workloadOnRetry := opts.OnRetry
	opts.OnRetry = func(attempt int, err error, decision RetryDecision) {
		if b.root != nil && b.root.errorReporter != nil {
			b.root.errorReporter.recordRetry(b.vu)
		}

		if workloadOnRetry != nil {
			workloadOnRetry(attempt, err, decision)
		}
	}

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

	actions := DefaultErrorActions()
	maps.Copy(actions, opts.Actions)

	return RetryPolicy{
		MaxAttempts:      opts.MaxAttempts,
		Classify:         classify,
		Actions:          actions,
		OnRetry:          opts.OnRetry,
		idempotent:       opts.Idempotent,
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
