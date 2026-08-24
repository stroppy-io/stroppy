package logger

import (
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	mysql "github.com/go-sql-driver/mysql"
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

func TestRedactDSNEmptyAndFallback(t *testing.T) {
	require.Empty(t, RedactDSN(""))
	require.Equal(t, redactedDSN, RedactDSN(redactedDSN))
}

func TestRedactDSNURI(t *testing.T) {
	dsn := "postgres://user:uri:secret@host:5432/db@name?sslmode=require" +
		"&client_secret=query-secret&client%255Fsecret=nested-secret" +
		"&x%2Dapi%2Dkey=api-secret&token=first-token&token=second-token&keep=yes#note@name"

	got := RedactDSN(dsn)
	require.NotEqual(t, redactedDSN, got)
	require.NotContains(t, got, "uri:secret")
	require.NotContains(t, got, "query-secret")
	require.NotContains(t, got, "nested-secret")
	require.NotContains(t, got, "api-secret")
	require.NotContains(t, got, "first-token")
	require.NotContains(t, got, "second-token")
	require.Equal(t, got, RedactDSN(got))

	parsed, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "host:5432", parsed.Host)
	require.Equal(t, "/db@name", parsed.Path)
	require.Equal(t, "note@name", parsed.Fragment)
	_, passwordPresent := parsed.User.Password()
	require.True(t, passwordPresent)

	password, _ := parsed.User.Password()
	require.Equal(t, redactedSecret, password)

	query, err := url.ParseQuery(parsed.RawQuery)
	require.NoError(t, err)
	require.Equal(t, "require", query.Get("sslmode"))
	require.Equal(t, "yes", query.Get("keep"))
	require.Equal(t, redactedSecret, query.Get("client_secret"))
	require.Equal(t, redactedSecret, query.Get("client%5Fsecret"))
	require.Equal(t, redactedSecret, query.Get("x-api-key"))
	require.Equal(t, []string{redactedSecret, redactedSecret}, query["token"])
}

func TestRedactDSNFallsBackForAmbiguousURI(t *testing.T) {
	for _, dsn := range []string{
		"postgres_://user:marker-secret@[2001:db8::1]:5432/db?sslmode=require",
		"postgres://user:marker-secret@host/db?bad=%zz",
		"postgres://user:marker-secret@host/db?client+secret=query-secret",
		"postgres://user:marker-secret@host/db?x=1;y=2",
	} {
		got := RedactDSN(dsn)
		require.Equal(t, redactedDSN, got)
		require.NotContains(t, got, "marker-secret")
		require.NotContains(t, got, "query-secret")
	}
}

func TestRedactDSNMySQL(t *testing.T) {
	tests := []struct {
		name          string
		dsn           string
		emptyPassword bool
		passwordless  bool
	}{
		{
			name: "password contains scheme separator",
			dsn:  "user:marker://secret@tcp(db:3306)/bench?keep=yes",
		},
		{
			name: "custom network",
			dsn:  "user:marker-secret@custom-net(db:3306)/bench?keep=yes",
		},
		{
			name: "encoded secret parameter",
			dsn:  "user:marker-secret@tcp(db:3306)/bench?client%5Fsecret=query-secret&keep=yes&charset=utf8",
		},
		{
			name:          "explicit empty password",
			dsn:           "user:@tcp(db:3306)/bench?charset=utf8",
			emptyPassword: true,
			passwordless:  true,
		},
		{
			name: "whitespace password",
			dsn:  "user:marker secret@tcp(db:3306)/bench?charset=utf8",
		},
		{
			name: "addressless protocol",
			dsn:  "user:marker-secret@tcp/bench?charset=utf8",
		},
		{
			name: "query punctuation in password",
			dsn:  "user:marker?secret@tcp(db:3306)/bench?charset=utf8",
		},
		{
			name: "Unix socket password",
			dsn:  "user:marker-secret@unix(/tmp/mysql.sock)/bench?charset=utf8",
		},
		{
			name:         "username only",
			dsn:          "user@tcp(db:3306)/bench?charset=utf8",
			passwordless: true,
		},
		{
			name:         "credential free address",
			dsn:          "tcp(db:3306)/bench?charset=utf8",
			passwordless: true,
		},
		{
			name:         "credential free custom network",
			dsn:          "custom-net(db:3306)/bench?charset=utf8",
			passwordless: true,
		},
		{
			name:         "credential free Unix socket",
			dsn:          "unix(/tmp/mysql.sock)/bench?charset=utf8",
			passwordless: true,
		},
		{
			name:         "credential free addressless network",
			dsn:          "tcp/bench?charset=utf8",
			passwordless: true,
		},
		{
			name:         "username only endpoint boundaries",
			dsn:          "user@tcp(db:3306)/bench@foo[bar?note=@foo[bar&charset=utf8",
			passwordless: true,
		},
		{
			name:         "credential free endpoint boundaries",
			dsn:          "tcp(db:3306)/bench@foo[bar?note=@foo[bar&charset=utf8",
			passwordless: true,
		},
		{
			name:         "username only query punctuation",
			dsn:          "user@tcp(db:3306)/bench?note=@foo%2Fbar&encoded=%2F%40&charset=utf8",
			passwordless: true,
		},
		{
			name:         "credential free query punctuation",
			dsn:          "tcp(db:3306)/bench?note=@foo%2Fbar&encoded=%2F%40&charset=utf8",
			passwordless: true,
		},
		{
			name:         "Unix socket punctuation",
			dsn:          "user@unix(/tmp/mysql))socket?file)/bench?charset=utf8",
			passwordless: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			original, err := mysql.ParseDSN(test.dsn)
			require.NoError(t, err)

			if test.emptyPassword {
				require.Empty(t, original.Passwd)
			}

			got := RedactDSN(test.dsn)
			require.NotEqual(t, redactedDSN, got)

			for _, marker := range []string{"marker-secret", "marker secret", "marker?secret", "query-secret"} {
				require.NotContains(t, got, marker)
			}

			require.Equal(t, got, RedactDSN(got))

			parsed, err := mysql.ParseDSN(got)
			require.NoError(t, err)
			require.Equal(t, original.User, parsed.User)
			require.Equal(t, original.Net, parsed.Net)
			require.Equal(t, original.Addr, parsed.Addr)
			require.Equal(t, original.DBName, parsed.DBName)

			if test.passwordless {
				require.Empty(t, original.Passwd)
				require.Empty(t, parsed.Passwd)
			} else {
				require.Equal(t, redactedSecret, parsed.Passwd)
			}

			for key, value := range original.Params {
				secret, certain := classifyQueryKey(key)
				require.True(t, certain)

				if secret {
					require.Equal(t, redactedSecret, parsed.Params[key])
				} else {
					require.Equal(t, value, parsed.Params[key])
				}
			}
		})
	}
}

func TestRedactDSNFallsBackForUncertainMySQLParam(t *testing.T) {
	dsn := "user:marker-secret@tcp(db:3306)/bench?client+secret=query-secret&keep=yes"
	got := RedactDSN(dsn)

	require.Equal(t, redactedDSN, got)
	require.NotContains(t, got, "marker-secret")
	require.NotContains(t, got, "query-secret")
}

func TestRedactDSNFallsBackForConninfoMySQLAmbiguity(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		secrets []string
	}{
		{
			name: "single-token overlap",
			dsn:  "password=marker-secret:other-secret@tcp(db:3306)/bench?keep=yes",
			secrets: []string{
				"marker-secret", "other-secret",
			},
		},
		{
			name: "equals username",
			dsn:  "bench=prod:marker-secret@tcp(db:3306)/bench?keep=yes",
			secrets: []string{
				"marker-secret",
			},
		},
		{
			name: "password-like username",
			dsn:  "password=account:marker-secret@tcp(db:3306)/bench?keep=yes",
			secrets: []string{
				"marker-secret",
			},
		},
		{
			name: "space-separated dual grammar",
			dsn:  "bench=prod:first-secret password=second-secret@tcp(db:3306)/bench?keep=yes",
			secrets: []string{
				"first-secret", "second-secret",
			},
		},
		{
			name: "tab-separated dual grammar",
			dsn:  "bench=prod:first-secret\tpassword=second-secret@tcp(db:3306)/bench?keep=yes",
			secrets: []string{
				"first-secret", "second-secret",
			},
		},
		{
			name: "newline-separated dual grammar",
			dsn:  "bench=prod:first-secret\npassword=second-secret@tcp(db:3306)/bench?keep=yes",
			secrets: []string{
				"first-secret", "second-secret",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RedactDSN(test.dsn)

			require.Equal(t, redactedDSN, got)

			for _, secret := range test.secrets {
				require.NotContains(t, got, secret)
			}

			require.Equal(t, got, RedactDSN(got))
		})
	}
}

func TestRedactDSNConninfo(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "duplicate secrets preserve outside spans",
			dsn:  "host=db application_name=worker password=first-secret sslmode=require token=second-secret",
			want: "host=db application_name=worker password=xxxxx sslmode=require token=xxxxx",
		},
		{
			name: "quoted token text is content not query",
			dsn:  "host=db application_name='?token=not-a-query' password='quoted secret' sslmode=require",
			want: "host=db application_name='?token=not-a-query' password=xxxxx sslmode=require",
		},
		{
			name: "UTF-8 bytes stay inside value",
			dsn:  "host=db password=pàss-marker sslmode=require",
			want: "host=db password=xxxxx sslmode=require",
		},
		{
			name: "ASCII whitespace only",
			dsn:  "host=db\tpassword=tab-secret\nsslmode=require",
			want: "host=db\tpassword=xxxxx\nsslmode=require",
		},
		{
			name: "escaped unquoted whitespace",
			dsn:  `host=db password=top\ secret sslmode=require`,
			want: "host=db password=xxxxx sslmode=require",
		},
		{
			name: "escaped quoted content",
			dsn:  `host=db password='top\'secret' sslmode=require`,
			want: "host=db password=xxxxx sslmode=require",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RedactDSN(test.dsn)
			require.Equal(t, test.want, got)
			require.Equal(t, got, RedactDSN(got))
			require.NotContains(t, got, "secret")
			require.NotContains(t, got, "pàss-marker")
		})
	}
}

func TestRedactDSNFallsBackForMalformedConninfo(t *testing.T) {
	for _, dsn := range []string{
		"host=db junk password=marker-secret sslmode=require",
		"junk host=db password=marker-secret sslmode=require",
		"host=db password=top\\",
		"host=db password='unterminated-secret",
	} {
		got := RedactDSN(dsn)
		require.Equal(t, redactedDSN, got)
		require.NotContains(t, got, "marker-secret")
		require.NotContains(t, got, "unterminated-secret")
	}
}

func TestRedactDSNInvariants(t *testing.T) {
	for index := range 32 {
		marker := "marker" + strconv.Itoa(index)
		for _, dsn := range []string{
			"user:" + marker + "@tcp(db:3306)/bench?token=" + marker,
			"host=db password=" + marker + " token=" + marker + " sslmode=require",
		} {
			got := RedactDSN(dsn)
			require.NotContains(t, got, marker)
			require.Equal(t, got, RedactDSN(got))
		}
	}
}

func TestRedactDSNNeverPanicsOnArbitraryBytes(t *testing.T) {
	inputs := []string{
		string([]byte{0xff, 0xfe, ':', '@', 0x00}),
		"host=db password=marker\x00secret",
		"postgres://user:marker-secret@host/\x00",
		strings.Repeat("!@:/?#%", 4096),
	}

	for _, input := range inputs {
		t.Run(strconv.Quote(input[:min(len(input), 16)]), func(t *testing.T) {
			require.NotPanics(t, func() { _ = RedactDSN(input) })
		})
	}
}
