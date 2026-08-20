package runner_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/internal/runner"
	"github.com/stroppy-io/stroppy/pkg/config"
)

func TestMergePostgresConfig(t *testing.T) {
	dst := &config.PostgresConfig{
		MaxConns:      ptr[int32](10),
		MinConns:      ptr[int32](2),
		TraceLogLevel: ptr("warn"),
	}

	src := &config.PostgresConfig{
		MaxConns:               ptr[int32](200),
		StatementCacheCapacity: ptr[int32](128),
	}

	merged := runner.MergePostgresConfig(dst, src)

	require.Equal(t, int32(200), merged.GetMaxConns())               // src overrides dst
	require.Equal(t, int32(2), merged.GetMinConns())                 // dst preserved
	require.Equal(t, "warn", merged.GetTraceLogLevel())              // dst preserved
	require.Equal(t, int32(128), merged.GetStatementCacheCapacity()) // src adds
	require.Nil(t, merged.MinIdleConns)                              // untouched from dst
}

func TestMergeSqlConfig(t *testing.T) {
	dst := &config.SqlConfig{
		MaxOpenConns:    ptr[int32](9),
		ConnMaxLifetime: ptr("1h"),
	}

	src := &config.SqlConfig{
		MaxOpenConns: ptr[int32](12),
	}

	merged := runner.MergeSqlConfig(dst, src)

	require.Equal(t, int32(12), merged.GetMaxOpenConns()) // src overrides
	require.Equal(t, "1h", merged.GetConnMaxLifetime())   // dst preserved
	require.Nil(t, merged.MaxIdleConns)                   // untouched
}

func TestMergeNilSourceIsNoop(t *testing.T) {
	dst := &config.PostgresConfig{MaxConns: ptr[int32](10)}
	sqlDst := &config.SqlConfig{MaxIdleConns: ptr[int32](3)}

	require.Same(t, dst, runner.MergePostgresConfig(dst, nil))
	require.Same(t, sqlDst, runner.MergeSqlConfig(sqlDst, nil))
}
