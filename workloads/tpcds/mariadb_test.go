package tpcds

import (
	"errors"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestMariaDBDefinitionAndStreams(t *testing.T) {
	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		driver bench.DriverTypeName
		want   error
	}{
		{bench.DriverMySQL, nil}, {bench.DriverPostgres, errMariaDBDriver}, {bench.DriverTiDB, errMariaDBDriver},
	} {
		_, err := catalog.ResolveRun(Test.Name, bench.RunOptions{
			Params:  bench.ParamInputs{CLI: map[string]string{"sql-dialect": "mariadb", "query-stream": "0"}},
			Drivers: map[string]bench.DriverConfig{"default": {Kind: test.driver}},
		})
		if !errors.Is(err, test.want) {
			t.Fatalf("%s error=%v, want %v", test.driver, err, test.want)
		}
	}

	w := &workload{driver: bench.DriverMySQL, sqlDialect: "mariadb", scaleFactor: 0.01, seed: 19620718}

	queries, err := w.streamQueries(0)
	if err != nil {
		t.Fatal(err)
	}

	if len(queries) != 103 {
		t.Fatalf("generated %d statements, want 103", len(queries))
	}

	for _, query := range queries {
		if strings.Contains(query.sql, "grouping(") {
			t.Fatalf("%s used MySQL-only grouping", query.name)
		}
	}
}
