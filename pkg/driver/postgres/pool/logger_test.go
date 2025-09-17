package pool

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/tracelog"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-core/pkg/logger"
)

func TestNewLoggerTracer(t *testing.T) {
	log := logger.Global()
	tracer, err := newLoggerTracer(log)
	require.NoError(t, err)
	require.NotNil(t, tracer)
}

func TestPgxExtLogger_Log(_ *testing.T) {
	log := zap.NewNop()
	pl := &pgxExtLogger{logger: log}
	ctx := context.Background()
	pl.Log(ctx, tracelog.LogLevelInfo, "test message", map[string]interface{}{"foo": "bar"})
}
