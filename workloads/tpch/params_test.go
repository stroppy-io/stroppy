package tpch

import (
	"reflect"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/postgres"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/sqldriver"
)

func TestQueryParametersMatchDialectSQL(t *testing.T) {
	t.Parallel()

	generated, err := streamParams(1, 19620719)
	if err != nil {
		t.Fatal(err)
	}

	for _, dialect := range []string{"pg.sql", "crdb.sql", "mysql.sql", "pico.sql", "ydb.sql"} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()

			text, readErr := files.ReadFile(dialect)
			if readErr != nil {
				t.Fatal(readErr)
			}

			sql := bench.ParseSQL(string(text))
			w := &workload{isPicodata: dialect == "pico.sql", needsEndDates: dialect == "pico.sql" || dialect == "ydb.sql"}

			for _, base := range []map[string]map[string]any{queryParams(1), generated} {
				before := w.prepareParams(base)
				params := w.prepareParams(base)

				for _, name := range queryNames {
					body, ok := sql.Query(name, "body")
					if !ok {
						t.Fatalf("missing query %s", name)
					}

					if _, _, bindErr := sqldriver.ProcessArgs(postgres.PgxDialect{}, body, params[name]); bindErr != nil {
						t.Errorf("%s: %v", name, bindErr)
					}

					params[name]["mutation"] = true
				}

				if !reflect.DeepEqual(before, w.prepareParams(base)) {
					t.Fatal("prepared parameters share mutable input maps")
				}
			}
		})
	}
}

func TestEndDateIntervals(t *testing.T) {
	t.Parallel()

	w := &workload{isPicodata: true, needsEndDates: true}

	params := w.buildParams()
	for name, expected := range map[string]map[string]any{
		"q1":  {"shipdate_cutoff": "1998-09-02"},
		"q3":  {"segment": "BUILDING", "date": "1995-03-15"},
		"q4":  {"date": "1993-07-01", "date_3m": "1993-10-01"},
		"q5":  {"region": "ASIA", "date": "1994-01-01", "date_1y": "1995-01-01"},
		"q12": {"shipmode1": "MAIL", "shipmode2": "SHIP", "date": "1994-01-01", "date_1y": "1995-01-01"},
		"q14": {"date": "1995-09-01", "date_1m": "1995-10-01"},
		"q15": {"date": "1996-01-01", "date_3m": "1996-04-01"},
	} {
		if !reflect.DeepEqual(params[name], expected) {
			t.Errorf("%s parameters = %v, want %v", name, params[name], expected)
		}
	}
}

func TestPicodataQueryOptions(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		rows, opcodes uint64
		want          string
	}{
		{0, 0, ""},
		{10000000, 0, "\nOPTION (sql_motion_row_max = 10000000)"},
		{0, 10000000000, "\nOPTION (sql_vdbe_opcode_max = 10000000000)"},
		{10000000, 10000000000, "\nOPTION (sql_motion_row_max = 10000000, sql_vdbe_opcode_max = 10000000000)"},
	} {
		if got := picodataQueryOptions(test.rows, test.opcodes); got != test.want {
			t.Errorf("options(%d, %d) = %q, want %q", test.rows, test.opcodes, got, test.want)
		}
	}
}

func TestPicodataQueryLimitsRequirePicodata(t *testing.T) {
	t.Parallel()

	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, driver := range []bench.DriverTypeName{bench.DriverPostgres, bench.DriverPicodata, bench.DriverYDB} {
		for _, flag := range []string{"pico-motion-row-max", "pico-vdbe-opcode-max"} {
			_, resolveErr := catalog.Resolve("tpch/tx", bench.ParamInputs{CLI: map[string]string{flag: "10000000"}},
				map[string]bench.DriverConfig{"": {Kind: driver}})
			if (resolveErr == nil) != (driver == bench.DriverPicodata) {
				t.Errorf("driver %s, %s: unexpected limit support: %v", driver, flag, resolveErr)
			}
		}
	}
}

func TestSQLFileDefaults(t *testing.T) {
	tests := []struct {
		driver bench.DriverTypeName
		want   string
	}{
		{bench.DriverPostgres, "pg.sql"},
		// TPC-H cannot run on CockroachDB without this: its pg.sql ends with a bare
		// ANALYZE, which CockroachDB rejects.
		{bench.DriverCRDB, "crdb.sql"},
		// YugabyteDB accepts the postgres asset unmodified, and ships none of its own.
		{bench.DriverYugabyte, "pg.sql"},
		{bench.DriverMySQL, "mysql.sql"},
		{bench.DriverTiDB, "mysql.sql"},
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
