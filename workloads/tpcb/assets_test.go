package tpcb

import (
	"testing"

	"github.com/stroppy-io/stroppy/v6/workloads/internal/workloadtest"
)

func TestEmbeddedAssetContract(t *testing.T) {
	workloadtest.Files(
		t,
		files,
		"README.md",
		"pg.sql",
		"crdb.sql",
		"mysql.sql",
		"tidb.sql",
		"pico.sql",
		"ydb.sql",
	)

	txQueries := make([]workloadtest.Query, 0, len(requiredTxQueries))
	for _, query := range requiredTxQueries {
		txQueries = append(txQueries, workloadtest.Query{Section: query.section, Name: query.query})
	}

	for _, dialect := range []string{"pg.sql", "crdb.sql", "mysql.sql", "tidb.sql", "pico.sql", "ydb.sql"} {
		t.Run(dialect, func(t *testing.T) {
			queries := txQueries
			// TiDB has no stored procedures, so its variant carries neither
			// create_procedures nor the workload_procs call.
			if dialect == "pg.sql" || dialect == "crdb.sql" || dialect == "mysql.sql" {
				queries = append(append([]workloadtest.Query(nil), txQueries...), workloadtest.Query{
					Section: "workload_procs",
					Name:    "tpcb_transaction",
				})
			}

			workloadtest.SQL(t, files, dialect, requiredSetupSections, queries)
		})
	}
}
