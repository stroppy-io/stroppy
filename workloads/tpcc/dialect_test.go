package tpcc

import (
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// TestEngineSpecificAssetIsSelected pins the same rule as tpcb's: an engine that
// speaks another protocol may still ship its own asset, and embedding it is the only
// way to offer one that does not need a path on disk.
func TestEngineSpecificAssetIsSelected(t *testing.T) {
	for _, testCase := range []struct {
		driver bench.DriverTypeName
		want   string
		why    string
	}{
		{bench.DriverPostgres, "pg.sql", "the protocol's own asset"},
		{bench.DriverCRDB, "crdb.sql", "CockroachDB needs per-table ANALYZE, and tpcc ships a crdb.sql for it"},
		{bench.DriverYugabyte, "pg.sql", "speaks the postgres protocol and ships no asset of its own, so it falls back"},
		{bench.DriverMySQL, "mysql.sql", "a different protocol"},
		{bench.DriverTiDB, "tidb.sql", "TiDB has no stored procedures, so tpcc ships a variant without them"},
		{bench.DriverPicodata, "pico.sql", "a different protocol"},
		{bench.DriverYDB, "ydb.sql", "a different protocol"},
	} {
		if got := sqlFile(testCase.driver, ""); got != testCase.want {
			t.Errorf("sqlFile(%s) = %q, want %q — %s", testCase.driver, got, testCase.want, testCase.why)
		}
	}

	if got := sqlFile(bench.DriverCRDB, "mine.sql"); got != "mine.sql" {
		t.Errorf("an operator's explicit override must win, got %q", got)
	}
}
