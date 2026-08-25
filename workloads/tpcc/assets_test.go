package tpcc

import (
	"testing"

	"github.com/stroppy-io/stroppy/workloads/internal/workloadtest"
)

func TestEmbeddedAssetContract(t *testing.T) {
	dialects := []string{"pg.sql", "mysql.sql", "pico.sql", "ydb.sql", "ydb_no_indexes.sql"}
	workloadtest.Files(t, files, append([]string{"README.md"}, dialects...)...)

	sections := []string{
		"drop_schema",
		"create_schema",
		"workload_tx_new_order",
		"workload_tx_payment",
		"workload_tx_order_status",
		"workload_tx_delivery",
		"workload_tx_stock_level",
	}
	queries := []workloadtest.Query{
		{Section: "workload_tx_new_order", Name: "get_customer"},
		{Section: "workload_tx_payment", Name: "get_customer_by_id"},
		{Section: "workload_tx_order_status", Name: "get_last_order"},
		{Section: "workload_tx_delivery", Name: "get_min_new_order"},
		{Section: "workload_tx_stock_level", Name: "get_district"},
	}

	for _, dialect := range dialects {
		dialect := dialect
		t.Run(dialect, func(t *testing.T) {
			dialectSections := sections
			dialectQueries := append([]workloadtest.Query(nil), queries...)
			if dialect == "pg.sql" || dialect == "mysql.sql" {
				dialectSections = append(append([]string(nil), sections...), "workload_procs")
				for _, name := range []string{"new_order", "payment", "order_status", "delivery", "stock_level"} {
					dialectQueries = append(dialectQueries, workloadtest.Query{Section: "workload_procs", Name: name})
				}
			}

			workloadtest.SQL(t, files, dialect, dialectSections, dialectQueries)
		})
	}
}
