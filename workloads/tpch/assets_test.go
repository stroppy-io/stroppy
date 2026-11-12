package tpch

import (
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/workloads/internal/workloadtest"
)

func TestYDBRowPartitionFloorDoesNotChangeColumnLayout(t *testing.T) {
	data, err := files.ReadFile("ydb.sql")
	if err != nil {
		t.Fatal(err)
	}

	sql := bench.ParseSQL(string(data))
	for _, query := range sql.Section("create_schema") {
		if strings.Contains(query, "AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64") {
			t.Fatal("row table retains deployment-sized partition floor")
		}
	}

	columns := strings.Join(sql.Section("create_schema_column"), "\n")
	if strings.Count(columns, "AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64") != 6 ||
		strings.Count(columns, "AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1") != 2 {
		t.Fatal("column table partition layout changed")
	}
}

func TestEmbeddedAssetContract(t *testing.T) {
	dialects := []string{"pg.sql", "crdb.sql", "mysql.sql", "pico.sql", "ydb.sql"}
	workloadtest.Files(
		t,
		files,
		append([]string{"README.md", "answers_sf1.json", "distributions.json"}, dialects...)...,
	)

	sections := map[string][]string{
		"pg.sql":    {"drop_schema", "create_schema", "set_unlogged", "create_indexes", "set_logged", "analyze"},
		"crdb.sql":  {"drop_schema", "create_schema", "set_unlogged", "create_indexes", "set_logged", "analyze"},
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

	// The MPP dialect supplies only the schema; its queries are pg.sql's.
	workloadtest.Files(t, files, "schema.gp.sql")
	workloadtest.SQL(t, files, "schema.gp.sql", []string{"drop_schema", "create_schema", "analyze"}, nil)
}
