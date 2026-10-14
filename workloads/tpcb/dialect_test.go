package tpcb

import (
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// TestEngineSpecificAssetIsSelected pins the rule that makes an engine's own asset
// reachable at all.
//
// TPC-B embeds a crdb.sql because CockroachDB rejects a bare ANALYZE. Before this,
// nothing could select it: workloads embed their assets, and the only other way to
// choose one is --sql-file, which takes a path on disk — so the shipped asset could
// only be used by copying it out of the binary first.
func TestEngineSpecificAssetIsSelected(t *testing.T) {
	for _, testCase := range []struct {
		driver bench.DriverTypeName
		want   string
		why    string
	}{
		{bench.DriverPostgres, "pg.sql", "the protocol's own asset"},
		{bench.DriverCRDB, "crdb.sql", "CockroachDB needs per-table ANALYZE, and tpcb ships a crdb.sql for it"},
		{bench.DriverYugabyte, "pg.sql", "speaks the postgres protocol and ships no asset of its own, so it falls back"},
		{bench.DriverMySQL, "mysql.sql", "a different protocol"},
		{bench.DriverTiDB, "mysql.sql", "speaks the mysql protocol, and tpcb ships no tidb.sql"},
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
