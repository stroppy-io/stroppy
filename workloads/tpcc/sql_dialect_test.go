package tpcc

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestEmbeddedDialectIgnoresLocalSQL(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile("crdb24.sql", []byte("invalid local SQL"), 0o600); err != nil {
		t.Fatal(err)
	}

	sql, err := loadEmbeddedSQL("crdb24")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(strings.Join(sql.Section("create_procedures"), "\n"), "WHILE loop_counter") {
		t.Fatal("selected SQL lacks the CRDB 24.1 procedure")
	}
}

func TestProcsRejectsProcedureFreeDialect(t *testing.T) {
	catalog, err := bench.NewCatalog(Tx, Procs)
	if err != nil {
		t.Fatal(err)
	}

	for _, dialect := range []string{"tidb", "pico", "ydb", "ydb_no_indexes"} {
		t.Run(dialect, func(t *testing.T) {
			inputs := bench.ParamInputs{CLI: map[string]string{"sql-dialect": dialect}}

			drivers := map[string]bench.DriverConfig{"": {Kind: bench.DriverMySQL}}
			if _, err := catalog.Resolve("tpcc/procs", inputs, drivers); !errors.Is(err, errProcsDriverUnsupported) {
				t.Fatalf("error=%v, want unsupported procedures", err)
			}

			if _, err := catalog.Resolve("tpcc/tx", inputs, drivers); err != nil {
				t.Fatalf("transaction variant rejected dialect: %v", err)
			}
		})
	}
}

func TestEmbeddedSQLDialectSelection(t *testing.T) {
	catalog, err := bench.NewCatalog(Tx, Procs)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		params map[string]string
		want   string
	}{
		{name: "default"},
		{name: "CRDB 24.1", params: map[string]string{"sql-dialect": "crdb24"}},
		{name: "invalid dialect", params: map[string]string{"sql-dialect": "missing"}, want: "sql-dialect"},
		{
			name:   "missing explicit file",
			params: map[string]string{"sql-file": t.TempDir() + "/missing.sql"},
			want:   "missing.sql",
		},
		{
			name:   "conflicting selectors",
			params: map[string]string{"sql-dialect": "crdb24", "sql-file": "custom.sql"},
			want:   "sql-dialect and sql-file cannot be combined",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := catalog.Resolve("tpcc/procs", bench.ParamInputs{CLI: tc.params},
				map[string]bench.DriverConfig{"": {Kind: bench.DriverCRDB}})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}
