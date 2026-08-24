package runner

import "github.com/stroppy-io/stroppy/pkg/config"

// UnmarshalStrict validates the full JSON token stream before decoding it.
func UnmarshalStrict(data []byte, v any) error {
	return config.Unmarshal(data, v)
}

// MergePostgresConfig merges src into dst and returns the result. Every field
// is optional (a pointer), so a non-nil src field overrides the dst field while
// a nil src field leaves dst unchanged.
func MergePostgresConfig(dst, src *config.PostgresConfig) *config.PostgresConfig {
	if dst == nil {
		return src
	}

	if src == nil {
		return dst
	}

	merged := *dst

	if src.TraceLogLevel != nil {
		merged.TraceLogLevel = src.TraceLogLevel
	}

	if src.MaxConnLifetime != nil {
		merged.MaxConnLifetime = src.MaxConnLifetime
	}

	if src.MaxConnIdleTime != nil {
		merged.MaxConnIdleTime = src.MaxConnIdleTime
	}

	if src.MaxConns != nil {
		merged.MaxConns = src.MaxConns
	}

	if src.MinConns != nil {
		merged.MinConns = src.MinConns
	}

	if src.MinIdleConns != nil {
		merged.MinIdleConns = src.MinIdleConns
	}

	if src.DefaultQueryExecMode != nil {
		merged.DefaultQueryExecMode = src.DefaultQueryExecMode
	}

	if src.DescriptionCacheCapacity != nil {
		merged.DescriptionCacheCapacity = src.DescriptionCacheCapacity
	}

	if src.StatementCacheCapacity != nil {
		merged.StatementCacheCapacity = src.StatementCacheCapacity
	}

	return &merged
}

// MergeSQLConfig merges src into dst with the same non-nil-overrides semantics
// as MergePostgresConfig, for the generic database/sql pool block.
func MergeSQLConfig(dst, src *config.SQLConfig) *config.SQLConfig {
	if dst == nil {
		return src
	}

	if src == nil {
		return dst
	}

	merged := *dst

	if src.MaxOpenConns != nil {
		merged.MaxOpenConns = src.MaxOpenConns
	}

	if src.MaxIdleConns != nil {
		merged.MaxIdleConns = src.MaxIdleConns
	}

	if src.ConnMaxLifetime != nil {
		merged.ConnMaxLifetime = src.ConnMaxLifetime
	}

	if src.ConnMaxIdleTime != nil {
		merged.ConnMaxIdleTime = src.ConnMaxIdleTime
	}

	return &merged
}
