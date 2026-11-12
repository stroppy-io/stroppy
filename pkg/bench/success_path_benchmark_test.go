package bench

import (
	"context"
	"testing"
)

func BenchmarkSuccessRetryPolicy(b *testing.B) {
	worker := &Bench{}
	ctx := context.Background()
	options := TxRetryPolicyOptions{MaxAttempts: 3}

	b.ReportAllocs()

	for b.Loop() {
		if err := Retry0(ctx, worker.TxRetryPolicy(options), func() error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
