package tpcds

import (
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/dsqgen"
)

// TestEngineKindsMapToTheirDialects pins #218: every driver kind must resolve to the
// same dialect in both places that choose one.
//
// Two paths broke when the engine kinds arrived. tpcds -d tidb selected the postgres
// assets instead of the MySQL ones, and the generated stream passed the kind itself —
// "tidb", "yugabyte" — to dsqgen, which knows only postgres and mysql, so explicit
// streams and throughput runs failed at define time.
func TestEngineKindsMapToTheirDialects(t *testing.T) {
	for _, tc := range []struct {
		driver bench.DriverTypeName
		schema string
	}{
		{bench.DriverPostgres, "schema.pg.sql"},
		{bench.DriverMySQL, "schema.mysql.sql"},
		{bench.DriverCRDB, "schema.pg.sql"},
		{bench.DriverYugabyte, "schema.pg.sql"},
		{bench.DriverTiDB, "schema.mysql.sql"},
		{bench.DriverYDB, "schema.ydb.sql"},
		{bench.DriverPicodata, "schema.pico.sql"},
	} {
		if got, _ := dialectFiles(tc.driver, "", ""); got != tc.schema {
			t.Errorf("dialectFiles(%s) schema = %q, want %q", tc.driver, got, tc.schema)
		}

		// The generated stream has to be given a name dsqgen knows. Handing it the
		// kind was the failure, so the name is checked against dsqgen itself rather
		// than against a list here.
		name := dsqgenDialect(tc.driver)
		if _, ok := dsqgen.DialectByName(name); !ok {
			t.Errorf("dsqgenDialect(%s) = %q, which dsqgen does not know", tc.driver, name)
		}
	}
}

// TestDsqgenDialectNeverPassesTheKind is the narrower assertion: no driver kind may
// reach dsqgen as itself, which is what the old code did for tidb and yugabyte.
func TestDsqgenDialectNeverPassesTheKind(t *testing.T) {
	for _, dt := range []bench.DriverTypeName{
		bench.DriverPostgres, bench.DriverMySQL, bench.DriverCRDB,
		bench.DriverYugabyte, bench.DriverTiDB, bench.DriverYDB, bench.DriverPicodata,
	} {
		if got := dsqgenDialect(dt); got != "postgres" && got != "mysql" {
			t.Errorf("dsqgenDialect(%s) = %q, want a dialect dsqgen knows", dt, got)
		}
	}
}
