package driver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultErrorFacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{name: "unknown", err: errors.New("boom"), want: ErrorKindUnknown},
		{name: "canceled", err: fmt.Errorf("wrapped: %w", context.Canceled), want: ErrorKindCanceled},
		{name: "timeout", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded), want: ErrorKindTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := DefaultErrorFacts(tt.err).Kind; got != tt.want {
				t.Fatalf("DefaultErrorFacts().Kind = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJoinErrorsDeduplicatesOnlyTopLevelCauses(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("shared cause")
	left := fmt.Errorf("left: %w", sentinel)
	right := fmt.Errorf("right: %w", sentinel)

	tests := []struct {
		name        string
		errs        []error
		wantCauses  []error
		wantMessage string
	}{
		{
			name:        "wrapper_then_sentinel",
			errs:        []error{left, sentinel},
			wantCauses:  []error{left},
			wantMessage: left.Error(),
		},
		{
			name:        "sentinel_then_wrapper",
			errs:        []error{sentinel, left},
			wantCauses:  []error{sentinel},
			wantMessage: sentinel.Error(),
		},
		{
			name:        "sibling_wrappers",
			errs:        []error{left, right},
			wantCauses:  []error{left, right},
			wantMessage: left.Error() + "\n" + right.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := JoinErrors(tt.errs...)
			if got.Error() != tt.wantMessage {
				t.Fatalf("JoinErrors message = %q, want %q", got.Error(), tt.wantMessage)
			}

			joined, ok := got.(interface{ Unwrap() []error })
			if !ok {
				t.Fatalf("JoinErrors result type %T has no multi-error unwrap", got)
			}

			causes := joined.Unwrap()
			if len(causes) != len(tt.wantCauses) {
				t.Fatalf("JoinErrors causes = %v, want %v", causes, tt.wantCauses)
			}

			for i := range causes {
				require.Same(t, tt.wantCauses[i], causes[i], "cause %d", i)
			}
		})
	}
}

func TestJoinErrorsReportsTimeoutOnce(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close rows")
	wrappedTimeout := fmt.Errorf("close: %w", context.DeadlineExceeded)
	got := JoinErrors(context.DeadlineExceeded, errors.Join(closeErr, wrappedTimeout))

	if !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("JoinErrors error = %v, want context deadline exceeded", got)
	}

	if !errors.Is(got, closeErr) {
		t.Fatalf("JoinErrors error = %v, want close error", got)
	}

	if count := strings.Count(got.Error(), context.DeadlineExceeded.Error()); count != 1 {
		t.Fatalf("deadline message count = %d, want 1: %v", count, got)
	}
}
