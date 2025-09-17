package logger

import (
	"context"

	"go.uber.org/zap"
)

func WithOptions(opts ...zap.Option) *zap.Logger {
	return Global().WithOptions(opts...)
}

func WithFields(fields ...zap.Field) *zap.Logger {
	return Global().With(fields...)
}

func Debug(msg string, fields ...zap.Field) {
	Global().Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	Global().Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	Global().Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	Global().Error(msg, fields...)
}

func DPanic(msg string, fields ...zap.Field) {
	Global().DPanic(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	Global().Panic(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	Global().Fatal(msg, fields...)
}

func DebugContext(ctx context.Context, msg string, fields ...zap.Field) {
	Global().Debug(msg, WithCtxFields(ctx, fields...)...)
}

func InfoContext(ctx context.Context, msg string, fields ...zap.Field) {
	Global().Info(msg, WithCtxFields(ctx, fields...)...)
}

func WarnContext(ctx context.Context, msg string, fields ...zap.Field) {
	Global().Warn(msg, WithCtxFields(ctx, fields...)...)
}

func ErrorContext(ctx context.Context, msg string, fields ...zap.Field) {
	Global().Error(msg, WithCtxFields(ctx, fields...)...)
}

func DPanicContext(ctx context.Context, msg string, fields ...zap.Field) {
	Global().DPanic(msg, WithCtxFields(ctx, fields...)...)
}

func PanicContext(ctx context.Context, msg string, fields ...zap.Field) {
	Global().Panic(msg, WithCtxFields(ctx, fields...)...)
}

func FatalContext(ctx context.Context, msg string, fields ...zap.Field) {
	Global().Fatal(msg, WithCtxFields(ctx, fields...)...)
}
