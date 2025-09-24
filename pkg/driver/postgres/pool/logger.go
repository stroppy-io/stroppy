package pool

import (
	"context"

	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
		pl.logger.Debug(msg, append(fields, zap.Stringer("PGX_LOG_LEVEL", level))...) //nolint: makezero // allow
	case tracelog.LogLevelDebug:
		pl.logger.Debug(msg, fields...)
	case tracelog.LogLevelInfo:
		pl.logger.Info(msg, fields...)
	case tracelog.LogLevelWarn:
		pl.logger.Warn(msg, fields...)
	case tracelog.LogLevelError:
		pl.logger.Error(msg, fields...)
	default:
		pl.logger.Warn(msg, append( //nolint: makezero // allow
			fields,
			zap.String("comment", "unavailable log level"),
			zap.Stringer("PGX_LOG_LEVEL", level),
		)...)
	}
}

func newLoggerTracer(logger *zap.Logger) (*tracelog.TraceLog, error) {
	levl, err := tracelog.LogLevelFromString(logger.Level().String())
	if err != nil {
		return nil, err
	}

	return &tracelog.TraceLog{
		Logger:   &pgxExtLogger{logger: logger.WithOptions(zap.AddCallerSkip(1))},
		LogLevel: levl,
		Config:   tracelog.DefaultTraceLogConfig(),
	}, nil
}
