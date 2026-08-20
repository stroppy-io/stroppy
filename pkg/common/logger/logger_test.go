package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestInit_ParsesLevelAndMode(t *testing.T) {
	t.Run("production info", func(t *testing.T) {
		require.NoError(t, Init("info", "production"))

		lg := Global()
		require.True(t, lg.Core().Enabled(zapcore.InfoLevel))
		require.False(t, lg.Core().Enabled(zapcore.DebugLevel))
	})

	t.Run("development debug", func(t *testing.T) {
		require.NoError(t, Init("debug", "development"))

		lg := Global()
		require.True(t, lg.Core().Enabled(zapcore.DebugLevel))
	})
}

func TestInit_RejectsInvalidLevelAndMode(t *testing.T) {
	require.Error(t, Init("verbose", "production"))
	require.Error(t, Init("info", "pretty"))
}

func TestParseMode(t *testing.T) {
	for _, valid := range []string{"development", "production"} {
		mode, err := ParseMode(valid)
		require.NoError(t, err)
		require.Equal(t, LogMod(valid), mode)
	}

	_, err := ParseMode("nope")
	require.Error(t, err)
}

func TestNewDefault_EnablesDebug(t *testing.T) {
	lg := newDefault()

	require.NotNil(t, lg)
	require.True(t, lg.Core().Enabled(zapcore.DebugLevel))
}

func TestNewZapCfg(t *testing.T) {
	tests := []struct {
		name     string
		mod      LogMod
		level    zapcore.Level
		expected zapcore.Level
	}{
		{name: "production", mod: ProductionMod, level: zapcore.InfoLevel, expected: zapcore.InfoLevel},
		{name: "development", mod: DevelopmentMod, level: zapcore.DebugLevel, expected: zapcore.DebugLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newZapCfg(tt.mod, tt.level)
			require.Equal(t, tt.expected, cfg.Level.Level())
			require.True(t, cfg.DisableStacktrace)
		})
	}
}

func TestGlobal_IsConfiguredInstance(t *testing.T) {
	require.NotNil(t, Global())
}

func TestNewStructLogger(t *testing.T) {
	lg := NewStructLogger("test")
	require.NotNil(t, lg)
	require.NotNil(t, lg.Named("child"))
}

func TestRedactDSN(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		expected string
	}{
		{
			name:     "postgres url with password",
			dsn:      "postgres://user:secret@localhost:5432/db",
			expected: "postgres://user:xxxxx@localhost:5432/db",
		},
		{
			name:     "postgres url without password",
			dsn:      "postgres://user@localhost:5432/db",
			expected: "postgres://user@localhost:5432/db",
		},
		{
			name:     "mysql dsn",
			dsn:      "user:secret@tcp(localhost:3306)/db",
			expected: "user:xxxxx@tcp(localhost:3306)/db",
		},
		{
			name:     "grpc url with password",
			dsn:      "grpc://admin:token@host:2136/database",
			expected: "grpc://admin:xxxxx@host:2136/database",
		},
		{
			name:     "no credentials",
			dsn:      "postgres://localhost:5432/db",
			expected: "postgres://localhost:5432/db",
		},
		{
			name:     "bare host no userinfo",
			dsn:      "localhost:5432/db",
			expected: "localhost:5432/db",
		},
		{
			name:     "empty",
			dsn:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, RedactDSN(tt.dsn))
		})
	}
}
