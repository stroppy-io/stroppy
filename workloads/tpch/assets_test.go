package tpch

import (
	"strconv"
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

	sections := []string{"drop_schema", "create_schema"}
	queries := make([]workloadtest.Query, 0, 22)

	for number := 1; number <= 22; number++ {
		section := "q" + strconv.Itoa(number)
		sections = append(sections, section)
		queries = append(queries, workloadtest.Query{Section: section, Name: "body"})
	}

	for _, dialect := range dialects {
		t.Run(dialect, func(t *testing.T) {
			workloadtest.SQL(t, files, dialect, sections, queries)
		})
	}
}
