package logger

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestNewDefault(t *testing.T) {
	lg := newDefault()

	require.NotNil(t, lg)
	require.True(t, lg.Core().Enabled(zapcore.DebugLevel))
}

func TestNewZapCfg(t *testing.T) {
	tests := []struct {
		name  string
		mode  LogMod
		level zapcore.Level
	}{
		{name: "production", mode: ProductionMod, level: zapcore.InfoLevel},
		{name: "development", mode: DevelopmentMod, level: zapcore.DebugLevel},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := newZapCfg(test.mode, test.level)
			require.Equal(t, test.level, cfg.Level.Level())
			require.True(t, cfg.DisableStacktrace)
		})
	}
}

func TestInitReplacesGlobal(t *testing.T) {
	require.NoError(t, Init("info", "production"))

	lg := Global()
	require.True(t, lg.Core().Enabled(zapcore.InfoLevel))
	require.False(t, lg.Core().Enabled(zapcore.DebugLevel))
}

func TestInitFailurePreservesGlobal(t *testing.T) {
	require.NoError(t, Init("warn", "development"))

	before := Global()

	require.Error(t, Init("verbose", "development"))
	require.Same(t, before, Global())

	require.Error(t, Init("warn", "pretty"))
	require.Same(t, before, Global())
}

func TestGlobalConcurrentReplacement(t *testing.T) {
	const (
		workers    = 32
		iterations = 100
	)

	var (
		wait   sync.WaitGroup
		failed atomic.Bool
	)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()

			for range iterations {
				if Global() == nil || Init("debug", "development") != nil {
					failed.Store(true)
				}
			}
		}()
	}

	wait.Wait()
	require.False(t, failed.Load())
	require.NotNil(t, Global())
}

func TestNewFromConfig(t *testing.T) {
	lg := NewFromConfig(&Config{LogMod: ProductionMod, LogLevel: "error"})

	require.Same(t, lg, Global())
	require.True(t, lg.Core().Enabled(zapcore.ErrorLevel))
	require.False(t, lg.Core().Enabled(zapcore.WarnLevel))
}

func TestParseMode(t *testing.T) {
	for _, mode := range []string{"development", "production"} {
		parsed, err := ParseMode(mode)
		require.NoError(t, err)
		require.Equal(t, LogMod(mode), parsed)
	}

	_, err := ParseMode("pretty")
	require.Error(t, err)
}

func TestNewStructLogger(t *testing.T) {
	lg := NewStructLogger("test")
	require.NotNil(t, lg)
}

func TestRedactDSN(t *testing.T) {
	secrets := []string{"dsn-secret", "db-pass", "query-token", "query-secret", "api-secret"}
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "url userinfo and repeated query secrets",
			dsn: "postgres://user:dsn-secret@host:5432/db?sslmode=require" +
				"&PASSWORD=db-pass&token=query-token&Token=query-secret" +
				"&api-key=api-secret&keep=yes",
			want: "postgres://user:xxxxx@host:5432/db?sslmode=require" +
				"&PASSWORD=xxxxx&token=xxxxx&Token=xxxxx&api-key=xxxxx&keep=yes",
		},
		{
			name: "mysql no scheme",
			dsn:  "user:dsn-secret@tcp(host:3306)/db?passwd=db-pass",
			want: "user:xxxxx@tcp(host:3306)/db?passwd=xxxxx",
		},
		{
			name: "encoded secret key",
			dsn:  "grpc://host/db?access%54oken=query-token&credentialS=query-secret",
			want: "grpc://host/db?access%54oken=xxxxx&credentialS=xxxxx",
		},
		{
			name: "malformed URL",
			dsn:  "postgres://user:dsn-secret@%zz/db?pwd=query-secret",
			want: "postgres://user:xxxxx@%zz/db?pwd=xxxxx",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RedactDSN(test.dsn)
			require.Equal(t, test.want, got)

			for _, secret := range secrets {
				require.NotContains(t, got, secret)
			}
		})
	}

	require.Equal(t, "postgres://host/db?sslmode=require", RedactDSN("postgres://host/db?sslmode=require"))
	require.Empty(t, RedactDSN(""))
}

func TestRedactDSNDoesNotLeakMalformedSecretKeys(t *testing.T) {
	got := RedactDSN("user:userinfo-secret@host/db?password%zz=malformed-secret&secret=still-secret")
	require.NotContains(t, got, "userinfo-secret")
	require.NotContains(t, got, "malformed-secret")
	require.NotContains(t, got, "still-secret")
	require.Contains(t, got, "password%zz=xxxxx")
}
