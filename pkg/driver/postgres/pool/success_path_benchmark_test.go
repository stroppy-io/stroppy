package pool

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func BenchmarkSuccessErrorTracing(b *testing.B) {
	core, _ := observer.New(zap.ErrorLevel)

	tracer, err := newLoggerTracer(zap.New(core))
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		acquired := tracer.TraceAcquireStart(ctx, nil, pgxpool.TraceAcquireStartData{})
		tracer.TraceAcquireEnd(acquired, nil, pgxpool.TraceAcquireEndData{})
		query := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
		tracer.TraceQueryEnd(query, nil, pgx.TraceQueryEndData{})
	}
}
