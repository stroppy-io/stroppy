package tpcds

import (
	"strconv"
	"testing"

	"github.com/stroppy-io/stroppy/workloads/internal/workloadtest"
)

func TestEmbeddedAssetContract(t *testing.T) {
	assets := []string{
		"README.md",
		"answers_sf1.json",
		"pg.sql",
		"mysql.sql",
		"pico.sql",
		"ydb.sql",
		"schema.pg.sql",
		"schema.mysql.sql",
		"schema.pico.sql",
		"schema.ydb.sql",
	}
	workloadtest.Files(t, files, assets...)

	schemas := map[string][]string{
		"schema.pg.sql":    {"create_schema", "create_indexes"},
		"schema.mysql.sql": {"create_schema", "create_indexes"},
		"schema.pico.sql":  {"drop_schema", "create_schema", "create_indexes"},
		"schema.ydb.sql":   {"drop_schema", "create_schema", "create_indexes"},
	}
	for name, sections := range schemas {
		name, sections := name, sections
		t.Run(name, func(t *testing.T) {
			workloadtest.SQL(t, files, name, sections, nil)
		})
	}

	queries := tpcdsContractQueries(nil)
	picoQueries := tpcdsContractQueries(map[int]bool{
		36: true,
		44: true,
		47: true,
		49: true,
		57: true,
		67: true,
		70: true,
		86: true,
	})
	for _, name := range []string{"pg.sql", "mysql.sql", "pico.sql", "ydb.sql"} {
		name := name
		t.Run(name, func(t *testing.T) {
			required := queries
			if name == "pico.sql" {
				required = picoQueries
			}
			workloadtest.SQL(t, files, name, nil, required)
		})
	}
}

func tpcdsContractQueries(skip map[int]bool) []workloadtest.Query {
	queries := make([]workloadtest.Query, 0, 103)
	for number := 1; number <= 99; number++ {
		if skip[number] {
			continue
		}

		name := "query_" + strconv.Itoa(number)
		switch number {
		case 14, 23, 24, 39:
			queries = append(queries, workloadtest.Query{Name: name + "_a"}, workloadtest.Query{Name: name + "_b"})
		default:
			queries = append(queries, workloadtest.Query{Name: name})
		}
	}

	return queries
}
