package ydb

import (
	"context"
	"errors"
	"fmt"
	"testing"

	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"
	ydbretry "github.com/ydb-platform/ydb-go-sdk/v3/retry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want driver.ErrorKind
	}{
		{name: "retryable", err: ydbretry.RetryableError(errors.New("transient")), want: driver.ErrorKindTransient},
		{name: "unsupported insert", err: ErrUnsupportedInsertMethod, want: driver.ErrorKindUnsupported},
		{name: "unsupported type", err: ErrUnsupportedType, want: driver.ErrorKindUnsupported},
		{name: "other", err: errors.New("boom"), want: driver.ErrorKindUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := (*Driver)(nil).ClassifyError(tt.err).Kind; got != tt.want {
				t.Fatalf("ClassifyError().Kind = %q, want %q", got, tt.want)
			}
		})
	}
}

// Use the SDK's real transport errors: a query stream can join its own
// cancellation with the server status that caused that cancellation.
func TestClassifyJoinedTransportError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want driver.ErrorFacts
	}{
		{
			"overload", ydbsdk.TransportError(status.Error(codes.ResourceExhausted, "ResourceExhausted")),
			driver.ErrorFacts{Kind: driver.ErrorKindTransient, Backoff: true},
		},
		{
			"unavailable", ydbsdk.TransportError(status.Error(codes.Unavailable, "connection lost")),
			driver.ErrorFacts{
				Kind: driver.ErrorKindTransient, Backoff: true,
				RequiresIdempotency: true,
			},
		},
		{
			name: "send too large",
			err:  ydbsdk.TransportError(status.Error(codes.ResourceExhausted, "trying to send message larger than max")),
			want: driver.ErrorFacts{Kind: driver.ErrorKindCanceled},
		},
		{
			name: "receive too large",
			err:  ydbsdk.TransportError(status.Error(codes.ResourceExhausted, "received message larger than max")),
			want: driver.ErrorFacts{Kind: driver.ErrorKindCanceled},
		},
		{
			"permission denied", ydbsdk.TransportError(status.Error(codes.PermissionDenied, "denied")),
			driver.ErrorFacts{Kind: driver.ErrorKindCanceled},
		},
		{"caller canceled", context.Canceled, driver.ErrorFacts{Kind: driver.ErrorKindCanceled}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := fmt.Errorf("tx query: %w", errors.Join(context.Canceled, tt.err))
			if got := (*Driver)(nil).ClassifyError(err); got != tt.want {
				t.Fatalf("ClassifyError() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRetryJoinedYDBOverload(t *testing.T) {
	t.Parallel()

	overload := errors.Join(context.Canceled,
		ydbsdk.TransportError(status.Error(codes.ResourceExhausted, "ResourceExhausted")))

	for _, cancelCaller := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancelCaller=%t", cancelCaller), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			attempts := 0

			err := bench.Retry0(ctx, bench.RetryPolicy{
				MaxAttempts: 3,
				Classify:    (*Driver)(nil).ClassifyError,
				Actions:     bench.DefaultErrorActions(),
			}, func() error {
				attempts++
				if attempts > 1 {
					return nil
				}

				if cancelCaller {
					cancel()
				}

				return overload
			})
			if cancelCaller {
				if !errors.Is(err, context.Canceled) || attempts != 1 {
					t.Fatalf("caller cancellation: err=%v, attempts=%d", err, attempts)
				}
			} else if err != nil || attempts != 2 {
				t.Fatalf("recoverable overload: err=%v, attempts=%d", err, attempts)
			}
		})
	}
}
