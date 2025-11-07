package logger

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

type contextKey string

const ctxLoggerKey contextKey = "logger"

// NewFromCtx creates new logger from context.
func NewFromCtx(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return Global()
	}

	if l, ok := ctx.Value(ctxLoggerKey).(*zap.Logger); ok {
		return l
	}

	return Global()
}

// WrapInCtx wraps logger in context.
func WrapInCtx(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey, logger)
}

const ctxLoggerAttrsKey contextKey = "context-logger-attrs"

func CtxWithAttrs(ctx context.Context, fields ...zap.Field) context.Context {
	logCtx := &loggerCtxAttrs{
		items: make([]zap.Field, 0),
	}

	logCtx.setAttrs(fields...)

	return context.WithValue(ctx, ctxLoggerAttrsKey, logCtx)
}

// SetCtxFields set key+value in passed context for logger.
func SetCtxFields(ctx context.Context, fields ...zap.Field) {
	logCtx, ok := ctx.Value(ctxLoggerAttrsKey).(*loggerCtxAttrs)
	if !ok {
		return
	}

	logCtx.setAttrs(fields...)
}

// GetCtxFields returns key+value from passed context for logger.
func GetCtxFields(ctx context.Context) []zap.Field {
	logCtx, ok := ctx.Value(ctxLoggerAttrsKey).(*loggerCtxAttrs)
	if !ok {
		return make([]zap.Field, 0)
	}

	return logCtx.getAttrs()
}

func WithCtxFields(ctx context.Context, fields ...zap.Field) []zap.Field {
	ctxFields := GetCtxFields(ctx)

	return append(ctxFields, fields...)
}

// loggerCtxAttrs private storage for logger's context.
type loggerCtxAttrs struct {
	mu    sync.RWMutex
	items []zap.Field
}

func (logCtx *loggerCtxAttrs) setAttrs(fields ...zap.Field) {
	logCtx.mu.Lock()
	defer logCtx.mu.Unlock()

	logCtx.items = append(logCtx.items, fields...)
}

func (logCtx *loggerCtxAttrs) getAttrs() []zap.Field {
	logCtx.mu.RLock()
	defer logCtx.mu.RUnlock()

	return logCtx.items
}
