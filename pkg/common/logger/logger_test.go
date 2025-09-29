package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestNewDefault(t *testing.T) {
	logger := newDefault()

	require.NotNil(t, logger)
	require.True(t, logger.Core().Enabled(zapcore.DebugLevel))
}

func TestNewZapCfg(t *testing.T) {
	tests := []struct {
		name     string
		mod      LogMod
		level    zapcore.Level
		expected zapcore.Level
	}{
		{
			name:     "Production mode",
			mod:      ProductionMod,
			level:    zapcore.InfoLevel,
			expected: zapcore.InfoLevel,
		},
		{
			name:     "Development mode",
			mod:      DevelopmentMod,
			level:    zapcore.DebugLevel,
			expected: zapcore.DebugLevel,
		},
		{
			name:     "Unknown mode",
			mod:      "unknown",
			level:    zapcore.WarnLevel,
			expected: zapcore.DebugLevel, // Default in development config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newZapCfg(tt.mod, tt.level)
			require.Equal(t, tt.expected, cfg.Level.Level())
		})
	}
}

func TestNewFromConfig(t *testing.T) {
	config := &Config{
		LogMod:   DevelopmentMod,
		LogLevel: zapcore.InfoLevel.String(),
	}
	logger := NewFromConfig(config)

	require.NotNil(t, logger)
	require.True(t, logger.Core().Enabled(zapcore.InfoLevel))
}

func TestGlobal(t *testing.T) {
	logger1 := Global()
	require.NotNil(t, logger1)

	// Set a new logger and test again
	config := &Config{
		LogMod:   ProductionMod,
		LogLevel: zapcore.WarnLevel.String(),
	}
	logger2 := NewFromConfig(config)
	require.Same(t, logger2, Global())

	require.True(t, logger2.Core().Enabled(zapcore.WarnLevel))
}

func TestNewStructLogger(t *testing.T) {
	logger := NewStructLogger("test")
	require.NotNil(t, logger)

	require.NotNil(t, logger.Named("test"))
}
