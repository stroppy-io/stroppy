package tpch

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/mpp"
)

// TestGreenplumSchemaPlacesEveryTable pins the property that makes the MPP schema
// worth having: no table is left to the server's default placement, and nothing needs
// a unique index, which an MPP engine cannot enforce unless it contains the distribution key.
func TestGreenplumSchemaPlacesEveryTable(t *testing.T) {
	t.Parallel()

	schema, err := files.ReadFile("schema.gp.sql")
	if err != nil {
		t.Fatal(err)
	}

	creates := regexp.MustCompile(`(?m)^CREATE TABLE (\w+) `).FindAllSubmatch(schema, -1)
	if len(creates) != len(tpchTables) {
		t.Fatalf("schema.gp.sql creates %d tables, want %d", len(creates), len(tpchTables))
	}

	placed := regexp.MustCompile(`(?m)^DISTRIBUTED (BY \(\w+\)|REPLICATED);$`).FindAll(schema, -1)
	if len(placed) != len(creates) {
		t.Errorf("%d tables, %d distribution clauses: every table needs exactly one", len(creates), len(placed))
	}

	statements := regexp.MustCompile(`(?m)^--.*$`).ReplaceAll(schema, nil)

	for _, banned := range []string{"PRIMARY KEY", "UNIQUE", "FOREIGN KEY", "CREATE INDEX"} {
		if regexp.MustCompile(`(?i)` + banned).Match(statements) {
			t.Errorf("schema.gp.sql contains %q", banned)
		}
	}
}

func TestDialectRequestsAreRefusedAtDefinition(t *testing.T) {
	t.Parallel()

	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	gp := func(extra ...string) map[string]string {
		params := map[string]string{"sql-dialect": "gp"}
		for i := 0; i < len(extra); i += 2 {
			params[extra[i]] = extra[i+1]
		}

		return params
	}

	for _, tc := range []struct {
		name   string
		driver bench.DriverTypeName
		params map[string]string
		want   error
	}{
		{"gp on pg", bench.DriverPostgres, gp(), nil},
		{"gp with an sql file", bench.DriverPostgres, gp("sql-file", "x.sql"), mpp.ErrFiles},
		{"gp with unlogged tables", bench.DriverPostgres, gp("pg-unlogged", "true"), mpp.ErrUnlog},
		{"gp on mysql", bench.DriverMySQL, gp(), mpp.ErrDriver},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := catalog.ResolveRun(Test.Name, bench.RunOptions{
				Params:  bench.ParamInputs{CLI: tc.params},
				Drivers: map[string]bench.DriverConfig{"default": {Kind: tc.driver}},
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}
