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
		{
			name:      "userinfo stays inside URL authority",
			dsn:       "postgres://user:dsn-secret@host:5432/db@name",
			want:      "postgres://user:xxxxx@host:5432/db@name",
			secrets:   []string{"dsn-secret"},
			preserved: []string{"host:5432/db@name"},
		},
		{
			name:      "userinfo does not scan URL fragment",
			dsn:       "postgres://user:dsn-secret@host:5432/db#note@name",
			want:      "postgres://user:xxxxx@host:5432/db#note@name",
			secrets:   []string{"dsn-secret"},
			preserved: []string{"host:5432/db#note@name"},
		},
		{
			name:      "userinfo does not scan no scheme path",
			dsn:       "user:dsn-secret@tcp(host:3306)/db@name",
			want:      "user:xxxxx@tcp(host:3306)/db@name",
			secrets:   []string{"dsn-secret"},
			preserved: []string{"tcp(host:3306)/db@name"},
		},
		{
			name: "PostgreSQL keyword secrets",
			dsn: "host=localhost user=bench password=dsn-secret sslmode=require" +
				" token=token-secret client_secret=client-secret application_name=worker:one@host",
			want: "host=localhost user=bench password=xxxxx sslmode=require" +
				" token=xxxxx client_secret=xxxxx application_name=worker:one@host",
			secrets:   []string{"dsn-secret", "token-secret", "client-secret"},
			preserved: []string{"host=localhost", "user=bench", "sslmode=require", "application_name=worker:one@host"},
		},
		{
			name: "quoted and escaped PostgreSQL keyword secrets",
			dsn: `host=localhost password='quoted secret' token='token\'secret' ` +
				`client_secret='client\ secret' application_name='bench run'`,
			want:      `host=localhost password=xxxxx token=xxxxx client_secret=xxxxx application_name='bench run'`,
			secrets:   []string{"quoted secret", `token\'secret`, `client\ secret`},
			preserved: []string{"host=localhost", "application_name='bench run'"},
		},
		{
			name:      "unterminated PostgreSQL secret value",
			dsn:       "host=localhost password='unterminated-secret sslmode=require",
			want:      "host=localhost password=xxxxx",
			secrets:   []string{"unterminated-secret"},
			preserved: []string{"host=localhost"},
		},
		{
			name:      "malformed PostgreSQL key value spacing",
			dsn:       "host=localhost password = dsn-secret sslmode=require",
			want:      "host=localhost password = xxxxx sslmode=require",
			secrets:   []string{"dsn-secret"},
			preserved: []string{"host=localhost", "sslmode=require"},
		},
		{
			name:      "URI password begins after first colon",
			dsn:       "postgres://u:pre:secret@host/db",
			want:      "postgres://u:xxxxx@host/db",
			secrets:   []string{"pre", "secret"},
			preserved: []string{"host/db"},
		},
		{
			name:      "PostgreSQL unquoted escaped whitespace",
			dsn:       `host=db password=top\ secret sslmode=require`,
			want:      "host=db password=xxxxx sslmode=require",
			secrets:   []string{`top\ secret`, "secret"},
			preserved: []string{"host=db", "sslmode=require"},
		},
		{
			name:      "PostgreSQL unquoted escaped backslash",
			dsn:       `host=db password=top\\secret sslmode=require`,
			want:      "host=db password=xxxxx sslmode=require",
			secrets:   []string{`top\\secret`, "secret"},
			preserved: []string{"host=db", "sslmode=require"},
		},
		{
			name:      "PostgreSQL malformed trailing escape",
			dsn:       `host=db password=top\`,
			want:      "host=db password=xxxxx",
			secrets:   []string{`top\`, "top"},
			preserved: []string{"host=db"},
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

func TestRedactDSNMySQLPasswordPunctuation(t *testing.T) {
	tests := []struct {
		name   string
		marker string
	}{
		{name: "colon", marker: "marker:colon"},
		{name: "slash", marker: "marker/slash"},
		{name: "at sign", marker: "marker@at"},
		{name: "question mark", marker: "marker?query"},
		{name: "fragment mark", marker: "marker#fragment"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := "user:" + test.marker + "@tcp(db:3306)/bench?charset=utf8"
			got := RedactDSN(dsn)

			require.Equal(t, "user:xxxxx@tcp(db:3306)/bench?charset=utf8", got)
			require.NotContains(t, got, test.marker)
			require.Contains(t, got, "tcp(db:3306)/bench")
			require.Contains(t, got, "charset=utf8")
		})
	}
}

func TestRedactDSNMySQLMalformedUserinfo(t *testing.T) {
	got := RedactDSN("user:marker-secret@tcp(db:3306")

	require.Equal(t, "user:xxxxx@tcp(db:3306", got)
	require.NotContains(t, got, "marker-secret")
	require.Contains(t, got, "tcp(db:3306")
}

func TestRedactDSNLayeredGrammarPasses(t *testing.T) {
	tests := []struct {
		name      string
		dsn       string
		want      string
		secrets   []string
		preserved []string
	}{
		{
			name:    "malformed scheme authority with IPv6",
			dsn:     "postgres_://user:marker-secret@[2001:db8::1]:5432/db?sslmode=require",
			want:    "postgres_://user:xxxxx@[2001:db8::1]:5432/db?sslmode=require",
			secrets: []string{"marker-secret"},
			preserved: []string{
				"[2001:db8::1]:5432/db", "sslmode=require",
			},
		},
		{
			name:    "MySQL username with equals",
			dsn:     "bench=prod:marker-secret@tcp(db:3306)/bench?charset=utf8",
			want:    "bench=prod:xxxxx@tcp(db:3306)/bench?charset=utf8",
			secrets: []string{"marker-secret"},
			preserved: []string{
				"bench=prod", "tcp(db:3306)/bench", "charset=utf8",
			},
		},
		{
			name:    "custom MySQL protocol",
			dsn:     "user:custom-secret@custom-net(db:3306)/bench?parseTime=true",
			want:    "user:xxxxx@custom-net(db:3306)/bench?parseTime=true",
			secrets: []string{"custom-secret"},
			preserved: []string{
				"custom-net(db:3306)/bench", "parseTime=true",
			},
		},
		{
			name:    "tcp4 MySQL protocol",
			dsn:     "user:tcp4-secret@tcp4(db:3306)/bench",
			want:    "user:xxxxx@tcp4(db:3306)/bench",
			secrets: []string{"tcp4-secret"},
			preserved: []string{
				"tcp4(db:3306)/bench",
			},
		},
		{
			name:    "tcp6 MySQL protocol",
			dsn:     "user:tcp6-secret@tcp6([::1]:3306)/bench",
			want:    "user:xxxxx@tcp6([::1]:3306)/bench",
			secrets: []string{"tcp6-secret"},
			preserved: []string{
				"tcp6([::1]:3306)/bench",
			},
		},
		{
			name:    "custom malformed MySQL protocol",
			dsn:     "user:custom-malformed-secret@custom-net(db:3306)",
			want:    "user:xxxxx@custom-net(db:3306)",
			secrets: []string{"custom-malformed-secret"},
			preserved: []string{
				"custom-net(db:3306)",
			},
		},
		{
			name:    "malformed MySQL protocol with equals username",
			dsn:     "bench=prod:malformed-secret@tcp[db:3306]/bench",
			want:    "bench=prod:xxxxx@tcp[db:3306]/bench",
			secrets: []string{"malformed-secret"},
			preserved: []string{
				"bench=prod", "tcp[db:3306]/bench",
			},
		},
		{
			name:    "malformed MySQL protocol",
			dsn:     "user:malformed-secret@tcp[db:3306]/bench",
			want:    "user:xxxxx@tcp[db:3306]/bench",
			secrets: []string{"malformed-secret"},
			preserved: []string{
				"tcp[db:3306]/bench",
			},
		},
		{
			name:    "leading malformed conninfo token",
			dsn:     "junk host=db password=marker-secret sslmode=require",
			want:    "junk host=db password=xxxxx sslmode=require",
			secrets: []string{"marker-secret"},
			preserved: []string{
				"junk", "host=db", "sslmode=require",
			},
		},
		{
			name:    "interspersed malformed conninfo token",
			dsn:     "host=db junk password=marker-secret sslmode=require",
			want:    "host=db junk password=xxxxx sslmode=require",
			secrets: []string{"marker-secret"},
			preserved: []string{
				"host=db", "junk", "sslmode=require",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RedactDSN(test.dsn)
			require.Equal(t, test.want, got)

			for _, secret := range test.secrets {
				require.NotContains(t, got, secret)
			}

			for _, value := range test.preserved {
				require.Contains(t, got, value)
			}
		})
	}
}
