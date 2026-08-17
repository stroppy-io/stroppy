package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestDefaultErrorActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind driver.ErrorKind
		want ErrorAction
	}{
		{name: "serialization", kind: driver.ErrorKindSerialization, want: ErrorActionRetry},
		{name: "deadlock", kind: driver.ErrorKindDeadlock, want: ErrorActionRetry},
		{name: "lock timeout", kind: driver.ErrorKindLockTimeout, want: ErrorActionRetry},
		{name: "transient", kind: driver.ErrorKindTransient, want: ErrorActionRetry},
		{name: "unknown", kind: driver.ErrorKindUnknown, want: ErrorActionError},
		{name: "unsupported", kind: driver.ErrorKindUnsupported, want: ErrorActionError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := DefaultErrorActions().action(tt.kind); got != tt.want {
				t.Fatalf("action(%q) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestErrorActionOverrides(t *testing.T) {
	t.Parallel()

	overrides := ErrorActionMap{ //nolint:exhaustive // test overrides selected kinds
		driver.ErrorKindSerialization: ErrorActionIgnore,
		driver.ErrorKindUnknown:       ErrorActionFatal,
	}
	policy := newTxRetryPolicy(nil, TxRetryPolicyOptions{Actions: overrides})

	if policy.Actions.action(driver.ErrorKindSerialization) != ErrorActionIgnore {
		t.Fatal("serialization override not applied")
	}

	if policy.Actions.action(driver.ErrorKindUnknown) != ErrorActionFatal {
		t.Fatal("unknown override not applied")
	}

	if policy.Actions.action(driver.ErrorKindDeadlock) != ErrorActionRetry {
		t.Fatal("default deadlock action not preserved")
	}
}

func TestRetryWithPolicyRetriesClassifiedError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("conflict")
	attempts := 0

	var retried []int

	policy := newTxRetryPolicy(
		func(error) driver.ErrorFacts { return driver.ErrorFacts{Kind: driver.ErrorKindSerialization} },
		TxRetryPolicyOptions{
			MaxAttempts: 3,
			OnRetry: func(attempt int, err error, decision RetryDecision) {
				if !errors.Is(err, sentinel) {
					t.Errorf("OnRetry error = %v, want sentinel", err)
				}

				if decision.Facts.Kind != driver.ErrorKindSerialization {
					t.Errorf("OnRetry kind = %q", decision.Facts.Kind)
				}

				retried = append(retried, attempt)
			},
		},
	)

	err := Retry0(context.Background(), policy, func() error {
		attempts++
		if attempts < 3 {
			return sentinel
		}

		return nil
	})
	if err != nil {
		t.Fatalf("Retry0() error = %v", err)
	}

	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}

	if len(retried) != 2 || retried[0] != 2 || retried[1] != 3 {
		t.Fatalf("retry attempts = %v, want [2 3]", retried)
	}
}

func TestRetryWithPolicyActions(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	tests := []struct {
		name      string
		action    ErrorAction
		wantErr   error
		wantFatal bool
	}{
		{name: "error", action: ErrorActionError, wantErr: sentinel},
		{name: "ignore", action: ErrorActionIgnore},
		{name: "fatal", action: ErrorActionFatal, wantErr: sentinel, wantFatal: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attempts := 0
			policy := newTxRetryPolicy(
				func(error) driver.ErrorFacts { return driver.ErrorFacts{Kind: driver.ErrorKindUnknown} },
				TxRetryPolicyOptions{
					MaxAttempts: 3,
					Actions: ErrorActionMap{ //nolint:exhaustive // test overrides one kind
						driver.ErrorKindUnknown: tt.action,
					},
				},
			)

			err := Retry0(context.Background(), policy, func() error {
				attempts++

				return sentinel
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Retry0() error = %v, want %v", err, tt.wantErr)
			}

			if IsFatalError(err) != tt.wantFatal {
				t.Fatalf("IsFatalError() = %v, want %v", IsFatalError(err), tt.wantFatal)
			}

			if attempts != 1 {
				t.Fatalf("attempts = %d, want 1", attempts)
			}
		})
	}
}

func TestRetryWithPolicyConditionalIdempotency(t *testing.T) {
	t.Parallel()

	classify := func(error) driver.ErrorFacts {
		return driver.ErrorFacts{
			Kind:                driver.ErrorKindTransient,
			RequiresIdempotency: true,
		}
	}

	for _, idempotent := range []bool{false, true} {
		t.Run(map[bool]string{false: "non-idempotent", true: "idempotent"}[idempotent], func(t *testing.T) {
			t.Parallel()

			attempts := 0
			policy := newTxRetryPolicy(classify, TxRetryPolicyOptions{MaxAttempts: 2, Idempotent: idempotent})
			err := Retry0(context.Background(), policy, func() error {
				attempts++
				if attempts == 1 {
					return errors.New("transient")
				}

				return nil
			})

			if idempotent && err != nil {
				t.Fatalf("Retry0() error = %v", err)
			}

			wantAttempts := 1
			if idempotent {
				wantAttempts = 2
			}

			if attempts != wantAttempts {
				t.Fatalf("attempts = %d, want %d", attempts, wantAttempts)
			}
		})
	}
}

func TestRetryWithPolicyCancellationDuringBackoff(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	policy := newTxRetryPolicy(
		func(error) driver.ErrorFacts {
			return driver.ErrorFacts{Kind: driver.ErrorKindTransient, Backoff: true}
		},
		TxRetryPolicyOptions{
			MaxAttempts:      3,
			BaseDelaySeconds: 10,
			MaxDelaySeconds:  10,
			OnRetry:          func(int, error, RetryDecision) { cancel() },
		},
	)

	attempts := 0

	err := Retry0(ctx, policy, func() error {
		attempts++

		return errors.New("transient")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Retry0() error = %v, want context.Canceled", err)
	}

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRetryWithPolicyMinimumAttempt(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	attempts := 0

	err := Retry0(context.Background(), RetryPolicy{}, func() error {
		attempts++

		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Retry0() error = %v, want sentinel", err)
	}

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
