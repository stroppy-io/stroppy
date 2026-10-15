package tpcds

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
)

func TestTypedParameterSemantics(t *testing.T) {
	unsetProcessEnv(
		t,
		"QUERY_STREAM",
		"VALIDATE_FORCE",
		"EXECUTOR",
		"VUS",
		"ITERATIONS",
		"DURATION",
	)

	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, inputs := range []bench.ParamInputs{
		{},
		{CLI: map[string]string{"query-stream": "0", "validate-force": "false"}},
		{WorkloadConfig: map[string]json.RawMessage{"validateForce": json.RawMessage(`false`)}},
	} {
		_, err := catalog.Resolve(
			"tpcds",
			inputs,
			map[string]bench.DriverConfig{"": {Kind: bench.DriverPostgres}},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestDialectFileDefaults(t *testing.T) {
	tests := []struct {
		driver      bench.DriverTypeName
		wantSchema  string
		wantQueries string
	}{
		{bench.DriverPostgres, "schema.pg.sql", "pg.sql"},
		{bench.DriverMySQL, "schema.mysql.sql", "mysql.sql"},
		{bench.DriverPicodata, "schema.pico.sql", "pico.sql"},
		{bench.DriverYDB, "schema.ydb.sql", "ydb.sql"},
	}
	for _, tt := range tests {
		schema, queries := dialectFiles(tt.driver, "", "")
		if schema != tt.wantSchema || queries != tt.wantQueries {
			t.Errorf("dialectFiles(%s) = (%s, %s), want (%s, %s)",
				tt.driver, schema, queries, tt.wantSchema, tt.wantQueries)
		}
	}

	schema, queries := dialectFiles(bench.DriverPostgres, "custom-schema.sql", "custom.sql")
	if schema != "custom-schema.sql" || queries != "custom.sql" {
		t.Fatalf("file overrides = (%s, %s), want custom files", schema, queries)
	}
}

func unsetProcessEnv(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		value, present := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() {
			if present {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}

// TestCockroachIsRefusedBeforeAnyWork pins the decision and its timing: TPC-DS cannot
// run on CockroachDB, because 11 of its 103 query bodies use ROLLUP, CUBE or GROUPING
// SETS, which CRDB does not implement. Refusing at define time rather than failing
// those queries mid-run matters because the load comes first — TPC-DS's tables are
// large, so the alternative spends minutes before saying the same thing.
func TestCockroachIsRefusedBeforeAnyWork(t *testing.T) {
	for _, testCase := range []struct {
		driver bench.DriverTypeName
		want   bool
		why    string
	}{
		{bench.DriverPostgres, true, "the workload's own dialect"},
		{bench.DriverMySQL, true, "a shipped dialect"},
		{bench.DriverPicodata, true, "a shipped dialect"},
		{bench.DriverYDB, true, "a shipped dialect"},
		{bench.DriverYugabyte, true, "speaks the postgres protocol"},
		{bench.DriverTiDB, true, "speaks the mysql protocol"},
		{bench.DriverCRDB, false, "ROLLUP/CUBE/GROUPING SETS are unimplemented (crdb#46280)"},
	} {
		if got := tpcdsSupported(testCase.driver); got != testCase.want {
			t.Errorf("tpcdsSupported(%s) = %v, want %v — %s", testCase.driver, got, testCase.want, testCase.why)
		}
	}
}
