package tpch

import (
	"testing"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

func TestSQLFileDefaults(t *testing.T) {
	tests := []struct {
		driver bench.DriverTypeName
		want   string
	}{
		{bench.DriverPostgres, "pg.sql"},
		{bench.DriverMySQL, "mysql.sql"},
		{bench.DriverPicodata, "pico.sql"},
		{bench.DriverYDB, "ydb.sql"},
	}
	for _, tt := range tests {
		if got := sqlFile(tt.driver, ""); got != tt.want {
			t.Errorf("sqlFile(%s) = %s, want %s", tt.driver, got, tt.want)
		}
	}

	if got := sqlFile(bench.DriverPostgres, "custom.sql"); got != "custom.sql" {
		t.Fatalf("SQL override = %s, want custom.sql", got)
	}
}
