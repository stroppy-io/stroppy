package tpcb

import (
	"testing"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

func TestLoadWorkersReachInsertRequests(t *testing.T) {
	const workers = 7

	requests := map[string]int{
		"branches": branchesRequest(1, workers).Workers,
		"tellers":  tellersRequest(1, workers).Workers,
		"accounts": accountsRequest(1, workers).Workers,
	}
	for name, got := range requests {
		if got != workers {
			t.Errorf("%s workers = %d, want %d", name, got, workers)
		}
	}
}

func TestDriverDerivedDefaults(t *testing.T) {
	isolationTests := []struct {
		driver bench.DriverTypeName
		want   bench.TxIsolationName
	}{
		{bench.DriverPostgres, bench.IsoReadCommitted},
		{bench.DriverMySQL, bench.IsoReadCommitted},
		{bench.DriverPicodata, bench.IsoNone},
		{bench.DriverYDB, bench.IsoSerializable},
	}
	for _, tt := range isolationTests {
		if got := resolveIsolation(tt.driver, ""); got != tt.want {
			t.Errorf("resolveIsolation(%s) = %s, want %s", tt.driver, got, tt.want)
		}
	}

	if got := resolveIsolation(bench.DriverPostgres, bench.IsoSerializable); got != bench.IsoSerializable {
		t.Fatalf("isolation override = %s, want %s", got, bench.IsoSerializable)
	}

	sqlTests := []struct {
		driver bench.DriverTypeName
		want   string
	}{
		{bench.DriverPostgres, "pg.sql"},
		{bench.DriverMySQL, "mysql.sql"},
		{bench.DriverPicodata, "pico.sql"},
		{bench.DriverYDB, "ydb.sql"},
	}
	for _, tt := range sqlTests {
		if got := sqlFile(tt.driver, ""); got != tt.want {
			t.Errorf("sqlFile(%s) = %s, want %s", tt.driver, got, tt.want)
		}
	}

	if got := sqlFile(bench.DriverPostgres, "custom.sql"); got != "custom.sql" {
		t.Fatalf("SQL override = %s, want custom.sql", got)
	}
}
