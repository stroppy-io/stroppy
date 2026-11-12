package tpcds

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/workloads/internal/workloadtest"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/dsqgen"
)

func TestYDBRowPartitionFloorDoesNotChangeColumnLayout(t *testing.T) {
	data, err := files.ReadFile("schema.ydb.sql")
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
	if strings.Count(columns, "AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64") != 11 ||
		strings.Count(columns, "AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1,") != 13 {
		t.Fatal("column table partition layout changed")
	}
}

func TestQualificationAssetsMatchGenerator(t *testing.T) {
	for name, dialect := range map[string]dsqgen.Dialect{
		"pg.sql": dsqgen.Postgres, "mysql.sql": dsqgen.MySQL, "mariadb.sql": dsqgen.MariaDB,
	} {
		result, err := dsqgen.Qualification(dialect)
		if err != nil {
			t.Fatal(err)
		}

		got, err := files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != dsqgen.QualificationSQL(dialect, result) {
			t.Fatalf("%s differs from fixed qualification values; run make gen-tpcds-qualification", name)
		}
	}
}

func TestQualificationPortParameters(t *testing.T) {
	for _, name := range []string{"pico.sql", "ydb.sql"} {
		data, err := files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}

		sql := strings.ToLower(string(data))

		_, rest, found := strings.Cut(sql, "--= query_96\n")
		if !found {
			t.Fatalf("%s missing query 96", name)
		}

		query, _, _ := strings.Cut(rest, "--= query_97\n")
		if !strings.Contains(query, "t_hour = 20") || !strings.Contains(query, "hd_dep_count = 7") {
			t.Fatalf("%s lost qualification query 96 substitutions", name)
		}

		if !strings.Contains(sql, "sum(sr_return_amt)") && !strings.Contains(sql, "sum(store_returns.sr_return_amt)") {
			t.Fatalf("%s lost qualification return aggregate", name)
		}

		if strings.Contains(sql, "2000-09-02") {
			t.Fatalf("%s retains a date bound from nonqualification substitutions", name)
		}
	}
}

func TestEmbeddedAssetContract(t *testing.T) {
	assets := []string{
		"README.md",
		"answers_sf1.json",
		"pg.sql",
		"mysql.sql",
		"mariadb.sql",
		"pico.sql",
		"ydb.sql",
		"schema.pg.sql",
		"schema.gp.sql",
		"schema.mysql.sql",
		"schema.pico.sql",
		"schema.ydb.sql",
	}
	workloadtest.Files(t, files, assets...)

	schemas := map[string][]string{
		"schema.pg.sql":    {"create_schema", "create_indexes"},
		"schema.gp.sql":    {"create_schema"},
		"schema.mysql.sql": {"create_schema", "create_indexes"},
		"schema.pico.sql":  {"drop_schema", "create_schema", "create_indexes"},
		"schema.ydb.sql":   {"drop_schema", "create_schema", "create_schema_column", "create_indexes"},
	}
	for name, sections := range schemas {
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

	for _, name := range []string{"pg.sql", "mysql.sql", "mariadb.sql", "pico.sql", "ydb.sql"} {
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
