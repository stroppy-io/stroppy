package tpcc

import (
	"context"
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestPopulationReadRetry(t *testing.T) {
	transient := errors.New("overloaded")

	permanent := errors.New("permission denied")
	for _, tc := range []struct {
		name      string
		failures  int
		failure   error
		wantCalls int
		wantErr   error
	}{
		{"recovered read", 2, transient, 3, nil},
		{"exhausted read", 4, transient, 3, transient},
		{"permanent error", 1, permanent, 1, permanent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, retries := 0, 0
			policy := bench.RetryPolicy{
				MaxAttempts: 3, Actions: bench.DefaultErrorActions(),
				Classify: func(err error) driver.ErrorFacts {
					if errors.Is(err, transient) {
						return driver.ErrorFacts{Kind: driver.ErrorKindTransient}
					}

					return driver.DefaultErrorFacts(err)
				},
				OnRetry: func(int, error, bench.RetryDecision) { retries++ },
			}

			got, err := readPopulation(t.Context(), policy, func() ([][]any, error) {
				calls++
				if calls <= tc.failures {
					return [][]any{{"partial stale row"}}, tc.failure
				}

				return [][]any{{int64(3001)}}, nil
			})
			if !errors.Is(err, tc.wantErr) || calls != tc.wantCalls || retries != calls-1 {
				t.Fatalf("err=%v calls=%d retries=%d", err, calls, retries)
			}

			if tc.wantErr != nil {
				if got != nil {
					t.Fatalf("failed read retained partial rows: %v", got)
				}
			} else if len(got) != 1 || got[0][0] != int64(3001) {
				t.Fatalf("recovered rows: %v", got)
			}
		})
	}
}

func TestPopulationReadCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	calls := 0
	policy := bench.RetryPolicy{
		MaxAttempts: 50, Actions: bench.DefaultErrorActions(),
		Classify: func(error) driver.ErrorFacts { return driver.ErrorFacts{Kind: driver.ErrorKindTransient} },
	}

	got, err := readPopulation(ctx, policy, func() (int, error) {
		calls++

		cancel()

		return 99, errors.New("stream overload after cancellation")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || got != 0 {
		t.Fatalf("got=%d err=%v calls=%d", got, err, calls)
	}
}
