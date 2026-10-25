package tpcds

import (
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// TestEngineKindsMapToTheirDialects pins both paths that choose a dialect, per kind.
//
// Two paths broke when the engine kinds arrived: tpcds -d tidb selected the postgres
// assets instead of the MySQL ones, and the generated stream passed the kind itself —
// "tidb", "yugabyte" — to dsqgen, which knows only postgres and mysql.
//
// The expected dialect is asserted exactly rather than merely checked against dsqgen,
// because "postgres" is a name dsqgen knows: a mapping that always returned postgres
// would satisfy the weaker check while silently sending TiDB down the wrong path.
func TestEngineKindsMapToTheirDialects(t *testing.T) {
	for _, tc := range []struct {
		driver  bench.DriverTypeName
		schema  string
		queries string
		dialect string
	}{
		{bench.DriverPostgres, "schema.pg.sql", "pg.sql", "postgres"},
		{bench.DriverMySQL, "schema.mysql.sql", "mysql.sql", "mysql"},
		{bench.DriverCRDB, "schema.pg.sql", "pg.sql", "postgres"},
		{bench.DriverYugabyte, "schema.pg.sql", "pg.sql", "postgres"},
		{bench.DriverTiDB, "schema.mysql.sql", "mysql.sql", "mysql"},
		{bench.DriverYDB, "schema.ydb.sql", "ydb.sql", "postgres"},
		{bench.DriverPicodata, "schema.pico.sql", "pico.sql", "postgres"},
	} {
		schema, queries := dialectFiles(tc.driver, "", "")
		if schema != tc.schema || queries != tc.queries {
			t.Errorf(
				"dialectFiles(%s) = %q, %q; want %q, %q",
				tc.driver, schema, queries, tc.schema, tc.queries,
			)
		}

		if got := dsqgenDialect(tc.driver); got != tc.dialect {
			t.Errorf("dsqgenDialect(%s) = %q, want %q", tc.driver, got, tc.dialect)
		}
	}
}

// TestGeneratedStreamsAreRefusedWhereTheGeneratorCannot is the second half of #218: a
// correct dialect name is not enough if the downstream generator cannot render the set.
// TiDB speaks mysql, whose generator skips q51 and q97; Picodata and YDB take streams
// the postgres generator produces with constructs their assets reject. All three must be
// refused at definition, before a load, rather than after one.
func TestGeneratedStreamsAreRefusedWhereTheGeneratorCannot(t *testing.T) {
	for _, tc := range []struct {
		driver bench.DriverTypeName
		want   bool
	}{
		{bench.DriverPostgres, true},
		{bench.DriverMySQL, true},
		{bench.DriverYugabyte, true},
		{bench.DriverTiDB, false},
		{bench.DriverPicodata, false},
		{bench.DriverYDB, false},
		{bench.DriverCRDB, false}, // refused earlier, by the TPC-DS support check
	} {
		if got := generatedStreamsSupported(tc.driver); got != tc.want {
			t.Errorf("generatedStreamsSupported(%s) = %v, want %v", tc.driver, got, tc.want)
		}
	}
}
