package common

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewLogger(t *testing.T) {
	delegate := zap.NewNop()
	logger := NewLogger(delegate)

	require.NotNil(t, logger)
	require.Equal(t, delegate, logger.delegate)
}

func TestLogger_convertLevel(t *testing.T) {
	logger := NewLogger(zap.NewNop())

	tests := []struct {
		name       string
		hclogLevel hclog.Level
		expected   zapcore.Level
	}{
		{"trace to debug", hclog.Trace, zapcore.DebugLevel},
		{"debug to debug", hclog.Debug, zapcore.DebugLevel},
		{"info to info", hclog.Info, zapcore.InfoLevel},
		{"warn to warn", hclog.Warn, zapcore.WarnLevel},
		{"error to error", hclog.Error, zapcore.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logger.convertLevel(tt.hclogLevel)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestLogger_convertLogLevel(t *testing.T) {
	logger := NewLogger(zap.NewNop())

	tests := []struct {
		name     string
		zapLevel zapcore.Level
		expected hclog.Level
	}{
		{"debug to debug", zapcore.DebugLevel, hclog.Debug},
		{"info to info", zapcore.InfoLevel, hclog.Info},
		{"warn to warn", zapcore.WarnLevel, hclog.Warn},
		{"error to error", zapcore.ErrorLevel, hclog.Error},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logger.convertLogLevel(tt.zapLevel)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestLogger_Log(t *testing.T) {
	core, obs := observer.New(zapcore.InfoLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	logger.Log(hclog.Info, "test message", "key", "value")

	require.Equal(t, 1, obs.Len())
	logEntry := obs.All()[0]
	require.Equal(t, "test message", logEntry.Message)
	require.Equal(t, zapcore.InfoLevel, logEntry.Level)
}

func TestLogger_Trace(t *testing.T) {
	core, obs := observer.New(zapcore.DebugLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	logger.Trace("trace message", "key", "value")

	require.Equal(t, 1, obs.Len())
	logEntry := obs.All()[0]
	require.Equal(t, "trace message", logEntry.Message)
	require.Equal(t, zapcore.DebugLevel, logEntry.Level)
}

func TestLogger_Debug(t *testing.T) {
	core, obs := observer.New(zapcore.DebugLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	logger.Debug("debug message", "key", "value")

	require.Equal(t, 1, obs.Len())
	logEntry := obs.All()[0]
	require.Equal(t, "debug message", logEntry.Message)
	require.Equal(t, zapcore.DebugLevel, logEntry.Level)
}

func TestLogger_Info(t *testing.T) {
	core, obs := observer.New(zapcore.InfoLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	logger.Info("info message", "key", "value")

	require.Equal(t, 1, obs.Len())
	logEntry := obs.All()[0]
	require.Equal(t, "info message", logEntry.Message)
	require.Equal(t, zapcore.InfoLevel, logEntry.Level)
}

func TestLogger_Warn(t *testing.T) {
	core, obs := observer.New(zapcore.WarnLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	logger.Warn("warn message", "key", "value")

	require.Equal(t, 1, obs.Len())
	logEntry := obs.All()[0]
	require.Equal(t, "warn message", logEntry.Message)
	require.Equal(t, zapcore.WarnLevel, logEntry.Level)
}

func TestLogger_Error(t *testing.T) {
	core, obs := observer.New(zapcore.ErrorLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	logger.Error("error message", "key", "value")

	require.Equal(t, 1, obs.Len())
	logEntry := obs.All()[0]
	require.Equal(t, "error message", logEntry.Message)
	require.Equal(t, zapcore.ErrorLevel, logEntry.Level)
}

func TestLogger_IsTrace(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	result := logger.IsTrace()
	require.True(t, result)

	core2, _ := observer.New(zapcore.InfoLevel)
	delegate2 := zap.New(core2)
	logger2 := NewLogger(delegate2)

	result2 := logger2.IsTrace()
	require.False(t, result2)
}

func TestLogger_IsDebug(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	result := logger.IsDebug()
	require.True(t, result)

	core2, _ := observer.New(zapcore.InfoLevel)
	delegate2 := zap.New(core2)
	logger2 := NewLogger(delegate2)

	result2 := logger2.IsDebug()
	require.False(t, result2)
}

func TestLogger_IsInfo(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	result := logger.IsInfo()
	require.True(t, result)

	core2, _ := observer.New(zapcore.WarnLevel)
	delegate2 := zap.New(core2)
	logger2 := NewLogger(delegate2)

	result2 := logger2.IsInfo()
	require.False(t, result2)
}

func TestLogger_IsWarn(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	result := logger.IsWarn()
	require.True(t, result)

	core2, _ := observer.New(zapcore.ErrorLevel)
	delegate2 := zap.New(core2)
	logger2 := NewLogger(delegate2)

	result2 := logger2.IsWarn()
	require.False(t, result2)
}

func TestLogger_IsError(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	result := logger.IsError()
	require.True(t, result)

	core2, _ := observer.New(zapcore.ErrorLevel)
	delegate2 := zap.New(core2)
	logger2 := NewLogger(delegate2)

	result2 := logger2.IsError()
	require.True(t, result2)
}

func TestLogger_ImpliedArgs(t *testing.T) {
	logger := NewLogger(zap.NewNop())

	result := logger.ImpliedArgs()
	require.Nil(t, result)
}

func TestLogger_With(t *testing.T) {
	core, obs := observer.New(zapcore.InfoLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	newLogger := logger.With("key", "value")
	require.NotNil(t, newLogger)
	require.NotEqual(t, logger, newLogger)

	newLogger.Info("test message")
	require.Equal(t, 1, obs.Len())
}

func TestLogger_Name(t *testing.T) {
	delegate := zap.NewNop().Named("test-logger")
	logger := NewLogger(delegate)

	result := logger.Name()
	require.Equal(t, "test-logger", result)
}

func TestLogger_Named(t *testing.T) {
	delegate := zap.NewNop()
	logger := NewLogger(delegate)

	newLogger := logger.Named("child-logger")
	require.NotNil(t, newLogger)
	require.NotEqual(t, logger, newLogger)
	require.Equal(t, "child-logger", newLogger.Name())
}

func TestLogger_ResetNamed(t *testing.T) {
	delegate := zap.NewNop().Named("original")
	logger := NewLogger(delegate)

	newLogger := logger.ResetNamed("new-name")
	require.NotNil(t, newLogger)
	require.NotEqual(t, logger, newLogger)
	require.Contains(t, newLogger.Name(), "new-name")
}

func TestLogger_SetLevel(t *testing.T) {
	core, obs := observer.New(zapcore.DebugLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	logger.SetLevel(hclog.Info)

	logger.Debug("debug message")
	require.Equal(t, 0, obs.Len())

	logger.Info("info message")
	require.Equal(t, 1, obs.Len())
}

func TestLogger_GetLevel(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	delegate := zap.New(core)
	logger := NewLogger(delegate)

	result := logger.GetLevel()
	require.Equal(t, hclog.Debug, result)

	core2, _ := observer.New(zapcore.InfoLevel)
	delegate2 := zap.New(core2)
	logger2 := NewLogger(delegate2)

	result2 := logger2.GetLevel()
	require.Equal(t, hclog.Info, result2)
}

func TestLogger_StandardLogger(t *testing.T) {
	delegate := zap.NewNop()
	logger := NewLogger(delegate)

	stdLogger := logger.StandardLogger(nil)
	require.NotNil(t, stdLogger)
}

func TestLogger_StandardWriter(t *testing.T) {
	delegate := zap.NewNop()
	logger := NewLogger(delegate)

	writer := logger.StandardWriter(nil)
	require.NotNil(t, writer)
}
