package common

import (
	"io"
	"log"

	"github.com/hashicorp/go-hclog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	delegate *zap.Logger
}

func NewLogger(delegate *zap.Logger) *Logger {
	return &Logger{delegate: delegate}
}

func (l *Logger) convertLevel(level hclog.Level) zapcore.Level {
	switch level {
	case hclog.Trace:
		return zapcore.DebugLevel
	case hclog.Debug:
		return zapcore.DebugLevel
	case hclog.Info:
		return zapcore.InfoLevel
	case hclog.Warn:
		return zapcore.WarnLevel
	case hclog.Error:
		return zapcore.ErrorLevel
	default:
		return zapcore.DebugLevel
	}
}

func (l *Logger) convertLogLevel(level zapcore.Level) hclog.Level {
	switch level {
	case zapcore.DebugLevel:
		return hclog.Debug
	case zapcore.InfoLevel:
		return hclog.Info
	case zapcore.WarnLevel:
		return hclog.Warn
	case zapcore.ErrorLevel:
		return hclog.Error
	default:
		return hclog.Debug
	}
}

func (l *Logger) Log(level hclog.Level, msg string, args ...interface{}) {
	l.delegate.Log(l.convertLevel(level), msg, zap.Any("_info", args))
}

func (l *Logger) Trace(msg string, args ...interface{}) {
	l.Log(hclog.Trace, msg, args...)
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.Log(hclog.Debug, msg, args...)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.Log(hclog.Info, msg, args...)
}

func (l *Logger) Warn(msg string, args ...interface{}) {
	l.Log(hclog.Warn, msg, args...)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.Log(hclog.Error, msg, args...)
}

func (l *Logger) IsTrace() bool {
	return l.delegate.Core().Enabled(zapcore.DebugLevel)
}

func (l *Logger) IsDebug() bool {
	return l.delegate.Core().Enabled(zapcore.DebugLevel)
}

func (l *Logger) IsInfo() bool {
	return l.delegate.Core().Enabled(zapcore.InfoLevel)
}

func (l *Logger) IsWarn() bool {
	return l.delegate.Core().Enabled(zapcore.WarnLevel)
}

func (l *Logger) IsError() bool {
	return l.delegate.Core().Enabled(zapcore.ErrorLevel)
}

func (l *Logger) ImpliedArgs() []interface{} {
	return nil
}

func (l *Logger) With(args ...interface{}) hclog.Logger { //nolint: ireturn // need from lib
	return &Logger{
		delegate: l.delegate.Sugar().With(args...).Desugar(),
	}
}

func (l *Logger) Name() string {
	return l.delegate.Name()
}

func (l *Logger) Named(name string) hclog.Logger { //nolint: ireturn // need from lib
	return &Logger{delegate: l.delegate.Named(name)}
}

func (l *Logger) ResetNamed(name string) hclog.Logger { //nolint: ireturn // need from lib
	return &Logger{delegate: l.delegate.Named(name)}
}

func (l *Logger) SetLevel(level hclog.Level) {
	l.delegate = l.delegate.WithOptions(zap.IncreaseLevel(l.convertLevel(level)))
}

func (l *Logger) GetLevel() hclog.Level { //nolint: ireturn // need from libLevel
	return l.convertLogLevel(l.delegate.Level())
}

func (l *Logger) StandardLogger(_ *hclog.StandardLoggerOptions) *log.Logger {
	return zap.NewStdLog(l.delegate)
}

func (l *Logger) StandardWriter(_ *hclog.StandardLoggerOptions) io.Writer {
	return zap.NewStdLog(l.delegate).Writer()
}
