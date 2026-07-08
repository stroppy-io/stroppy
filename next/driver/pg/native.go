package pg

import (
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/next/driver"
)

// NativeConfig carries the pgx-specific advanced knobs (D2 class B) a pg slot
// honors. They are opaque to the SDK; the bench layer writes them into
// [driver.Spec.Native] and pg reads them via [Native].
type NativeConfig struct {
	// ServerPrepare selects the extended (true, default) vs simple (false)
	// query path.
	//
	// When true (PerVU), a prepared handle reuses a server-side statement by
	// name; under Shared acquisition pgx's automatic statement cache handles
	// the equivalence per borrowed connection.
	//
	// When false, the connection runs SQL text directly with no server-side
	// prepare — v5's behavior (v5 sent parameterized text, never prepared).
	ServerPrepare bool
	// DefaultQueryExecMode overrides the pgx query execution mode. Empty leaves
	// pgx's default, except ServerPrepare=false maps to "simple" for v5 parity.
	// Recognized values: cache_statement, cache_describe, describe_exec, exec,
	// simple. See [pgx.QueryExecMode].
	DefaultQueryExecMode string
}

// Native reads pg's class-B knobs from spec. Missing entries take pg's defaults
// (server-prepare on; pgx's default exec mode). It is the single typed accessor
// the bench/probe layer consults; the SDK treats Native as opaque.
func Native(spec driver.Spec) NativeConfig {
	nc := NativeConfig{ServerPrepare: true}
	if spec.Native == nil {
		return nc
	}
	if v, ok := spec.Native["server_prepare"]; ok {
		if b, ok := toBool(v); ok {
			nc.ServerPrepare = b
		}
	}
	if v, ok := spec.Native["default_query_exec_mode"]; ok {
		if s, ok := v.(string); ok {
			nc.DefaultQueryExecMode = strings.ToLower(s)
		}
	}
	return nc
}

// toBool coerces the common JSON/Go scalar forms a Native map value carries to
// a bool. A Native map is map[string]any, so a JSON literal lands here as bool
// (after encoding/json) or int (1/0); a Go caller may also pass either.
func toBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case int:
		return x != 0, true
	case int64:
		return x != 0, true
	case string:
		switch strings.ToLower(x) {
		case "true", "1", "on", "yes":
			return true, true
		case "false", "0", "off", "no":
			return false, true
		}
	}
	return false, false
}

// resolveExecMode maps nc to a pgx execution mode. An explicit
// DefaultQueryExecMode wins; otherwise ServerPrepare=false selects the simple
// protocol for v5 parity, and ServerPrepare=true leaves pgx's default (unset,
// which pgx resolves to CacheStatement).
func resolveExecMode(nc NativeConfig) (pgx.QueryExecMode, bool) {
	switch nc.DefaultQueryExecMode {
	case "cache_statement":
		return pgx.QueryExecModeCacheStatement, true
	case "cache_describe":
		return pgx.QueryExecModeCacheDescribe, true
	case "describe_exec":
		return pgx.QueryExecModeDescribeExec, true
	case "exec":
		return pgx.QueryExecModeExec, true
	case "simple":
		return pgx.QueryExecModeSimpleProtocol, true
	case "":
		if !nc.ServerPrepare {
			return pgx.QueryExecModeSimpleProtocol, true
		}
		return 0, false
	}
	return 0, false
}
