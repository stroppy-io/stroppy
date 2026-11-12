package pool

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestErrorTracerKeepsQueryAndAcquireFailures(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.ErrorLevel)
	tracer, err := newLoggerTracer(zap.New(core))
	require.NoError(t, err)

	failure := errors.New("backend failure")
	ctx := tracer.TraceQueryStart(t.Context(), nil, pgx.TraceQueryStartData{SQL: "SELECT :id", Args: []any{42}})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: failure})
	acquireCtx := tracer.TraceAcquireStart(t.Context(), nil, pgxpool.TraceAcquireStartData{})
	tracer.TraceAcquireEnd(acquireCtx, nil, pgxpool.TraceAcquireEndData{Err: failure})
	require.Len(t, logs.All(), 2)
	require.Equal(t, "SELECT :id", logs.All()[0].ContextMap()["sql"])
	require.Equal(t, []any{42}, logs.All()[0].ContextMap()["args"])
	require.Contains(t, logs.All()[0].ContextMap(), "time")
	require.Equal(t, "Acquire", logs.All()[1].Message)
}

func TestErrorTracerWarmSuccessAllocations(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	tracer, err := newLoggerTracer(zap.New(core))
	require.NoError(t, err)

	ctx := context.Background()
	query := pgx.TraceQueryStartData{SQL: "SELECT 1"}
	tracer.TraceQueryStart(ctx, nil, query)
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	allocations := testing.AllocsPerRun(100, func() {
		tracer.TraceQueryStart(ctx, nil, query)
		tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
	})
	require.Zero(t, allocations)
	require.Empty(t, logs.All())
}

func TestErrorArgumentsRemainBounded(t *testing.T) {
	t.Parallel()

	values := errorQueryArgs([]any{strings.Repeat("a", 100), []byte(strings.Repeat("b", 100)), 42})
	require.Equal(t, strings.Repeat("a", 64)+" (truncated 36 bytes)", values[0])
	require.Equal(t, strings.Repeat("62", 64)+" (truncated 36 bytes)", values[1])
	require.Equal(t, 42, values[2])
}
