package driver

import (
	"context"
	"errors"
	"fmt"
	"testing"
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
