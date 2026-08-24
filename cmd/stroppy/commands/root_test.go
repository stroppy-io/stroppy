package commands

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestExitCodeFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		cancelCode int
		err        error
		want       int
	}{
		{"success", 130, nil, 0},
		{"canceled sigint", 130, context.Canceled, 130},
		{"canceled sigterm", 143, context.Canceled, 143},
		{"wrapped canceled", 130, fmt.Errorf("run: %w", context.Canceled), 130},
		{"other error", 130, errors.New("boom"), 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := exitCodeFor(tc.cancelCode, tc.err); got != tc.want {
				t.Fatalf("exitCodeFor(%d, %v) = %d, want %d", tc.cancelCode, tc.err, got, tc.want)
			}
		})
	}
}
