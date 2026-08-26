package tpch

import (
	"testing"

	"github.com/stroppy-io/stroppy/workloads/internal/workloadtest"
)

func TestEmbeddedAssetContract(t *testing.T) {
	dialects := []string{"pg.sql", "mysql.sql", "pico.sql", "ydb.sql"}
	workloadtest.Files(
		t,
		files,
		append([]string{"README.md", "answers_sf1.json", "distributions.json"}, dialects...)...,
	)

	sections := map[string][]string{
		"pg.sql":    {"drop_schema", "create_schema", "set_unlogged", "create_indexes", "set_logged", "analyze"},
		"mysql.sql": {"drop_schema", "create_schema", "create_indexes", "analyze"},
		"pico.sql":  {"drop_schema", "create_schema", "create_indexes"},
		"ydb.sql":   {"drop_schema", "create_schema", "create_schema_column", "create_indexes"},
	}
	queries := make([]workloadtest.Query, 0, len(queryNames))

	for _, section := range queryNames {
		queries = append(queries, workloadtest.Query{Section: section, Name: "body"})
	}

	for _, dialect := range dialects {
		t.Run(dialect, func(t *testing.T) {
			workloadtest.SQL(t, files, dialect, sections[dialect], queries)
		})
	}
}
