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
	tests := []struct {
		name      string
		dsn       string
		want      string
		secrets   []string
		preserved []string
	}{
		{
			name: "userinfo and named query secrets",
			dsn: "postgres://user:dsn-secret@host:5432/db?sslmode=require" +
				"&PASSWORD=db-pass&token=query-token&Token=query-secret" +
				"&api-key=api-secret&keep=yes",
			want: "postgres://user:xxxxx@host:5432/db?sslmode=require" +
				"&PASSWORD=xxxxx&token=xxxxx&Token=xxxxx&api-key=xxxxx&keep=yes",
			secrets:   []string{"dsn-secret", "db-pass", "query-token", "query-secret", "api-secret"},
			preserved: []string{"sslmode=require", "keep=yes"},
		},
		{
			name: "secret suffix families",
			dsn: "postgres://host/db?password=password-secret&passwd=passwd-secret&pwd=pwd-secret" +
				"&dbPassword=db-password&db_password=db-underscore-password" +
				"&clientSecret=client-secret&client_secret=client-underscore-secret" +
				"&oauth_client_secret=oauth-secret&credential=credential-secret&credentials=credentials-secret" +
				"&apiKey=api-key-secret&x-api-key=x-api-secret" +
				"&access_token=access-secret&refreshToken=refresh-secret&auth-token=auth-secret" +
				"&application_name=stroppy&sslmode=require",
			want: "postgres://host/db?password=xxxxx&passwd=xxxxx&pwd=xxxxx" +
				"&dbPassword=xxxxx&db_password=xxxxx" +
				"&clientSecret=xxxxx&client_secret=xxxxx" +
				"&oauth_client_secret=xxxxx&credential=xxxxx&credentials=xxxxx" +
				"&apiKey=xxxxx&x-api-key=xxxxx" +
				"&access_token=xxxxx&refreshToken=xxxxx&auth-token=xxxxx" +
				"&application_name=stroppy&sslmode=require",
			secrets: []string{
				"password-secret", "passwd-secret", "pwd-secret", "db-password", "db-underscore-password",
				"client-secret", "client-underscore-secret", "oauth-secret", "credential-secret", "credentials-secret",
				"api-key-secret", "x-api-secret", "access-secret", "refresh-secret", "auth-secret",
			},
			preserved: []string{"application_name=stroppy", "sslmode=require"},
		},
		{
			name: "encoded separators case variants and repeated values",
			dsn: "grpc://host/db?CLIENT%5fSECRET=client-secret" +
				"&oauth%255Fclient%255Fsecret=oauth-secret&refresh%2Dtoken=refresh-secret" +
				"&x%2Dapi%2Dkey=api-secret&Token=first-token&token=second-token" +
				"&keep=yes&sslmode=require",
			want: "grpc://host/db?CLIENT%5fSECRET=xxxxx" +
				"&oauth%255Fclient%255Fsecret=xxxxx&refresh%2Dtoken=xxxxx" +
				"&x%2Dapi%2Dkey=xxxxx&Token=xxxxx&token=xxxxx" +
				"&keep=yes&sslmode=require",
			secrets: []string{
				"client-secret", "oauth-secret", "refresh-secret", "api-secret", "first-token", "second-token",
			},
			preserved: []string{"keep=yes", "sslmode=require"},
		},
		{
			name: "malformed no scheme",
			dsn: "user:userinfo-secret@tcp(host:3306)/db?db%5Fpassword%zz=db-password" +
				"&client%_secret=client-secret&password%zz=password-secret&keep=yes&charset=utf8",
			want: "user:xxxxx@tcp(host:3306)/db?db%5Fpassword%zz=xxxxx" +
				"&client%_secret=xxxxx&password%zz=xxxxx&keep=yes&charset=utf8",
			secrets:   []string{"userinfo-secret", "db-password", "client-secret", "password-secret"},
			preserved: []string{"keep=yes", "charset=utf8"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RedactDSN(test.dsn)
			require.Equal(t, test.want, got)

			for _, secret := range test.secrets {
				require.NotContains(t, got, secret)
			}

			for _, option := range test.preserved {
				require.Contains(t, got, option)
			}
		})
	}

	require.Equal(t, "postgres://host/db?sslmode=require", RedactDSN("postgres://host/db?sslmode=require"))
	require.Empty(t, RedactDSN(""))
}
