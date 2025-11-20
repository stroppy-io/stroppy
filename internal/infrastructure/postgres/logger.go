package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
)

type pgxExtLogger struct {
	logger *zap.Logger
}

func (pl *pgxExtLogger) Log(_ context.Context, level tracelog.LogLevel, msg string, data map[string]interface{}) {
	fields := make([]zapcore.Field, len(data))
	i := 0
	for k, v := range data {
		fields[i] = zap.Any(k, v)
		i++
	}

	switch level {
	case tracelog.LogLevelTrace:
		pl.logger.Debug(msg, append(fields, zap.Stringer("PGX_LOG_LEVEL", level))...)
	case tracelog.LogLevelDebug:
		pl.logger.Debug(msg, fields...)
	case tracelog.LogLevelInfo:
		pl.logger.Info(msg, fields...)
	case tracelog.LogLevelWarn:
		pl.logger.Warn(msg, fields...)
	case tracelog.LogLevelError:
		pl.logger.Error(msg, fields...)
	default:
		pl.logger.Warn(msg, append(fields, zap.String("comment", "unavailable log level"), zap.Stringer("PGX_LOG_LEVEL", level))...)
	}
}

func newLoggerTracer(level string) (*tracelog.TraceLog, error) {
	levl, err := tracelog.LogLevelFromString(level)
	if err != nil {
		return nil, err
	}
	lg := &pgxExtLogger{logger: logger.WithOptions(zap.AddCallerSkip(1))}
	return &tracelog.TraceLog{
		Logger:   lg,
		LogLevel: levl,
		Config:   tracelog.DefaultTraceLogConfig(),
	}, nil
}
