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
		{Section: "workload_tx_new_order", Name: "get_warehouse"},
		{Section: "workload_tx_new_order", Name: "get_district"},
		{Section: "workload_tx_new_order", Name: "update_district"},
		{Section: "workload_tx_new_order", Name: "insert_order"},
		{Section: "workload_tx_new_order", Name: "insert_new_order"},
		{Section: "workload_tx_new_order", Name: "get_items_batch"},
		{Section: "workload_tx_new_order", Name: "get_stocks_batch"},
		{Section: "workload_tx_new_order", Name: "update_stock"},
		{Section: "workload_tx_new_order", Name: "insert_order_line"},
		{Section: "workload_tx_payment", Name: "count_customers_by_name"},
		{Section: "workload_tx_payment", Name: "get_customer_by_name"},
		{Section: "workload_tx_payment", Name: "get_customer_by_id"},
		{Section: "workload_tx_payment", Name: "update_customer_bc"},
		{Section: "workload_tx_payment", Name: "update_customer"},
		{Section: "workload_tx_payment", Name: "insert_history"},
		{Section: "workload_tx_order_status", Name: "count_customers_by_name"},
		{Section: "workload_tx_order_status", Name: "get_customer_by_name"},
		{Section: "workload_tx_order_status", Name: "get_customer_by_id"},
		{Section: "workload_tx_order_status", Name: "get_last_order"},
		{Section: "workload_tx_order_status", Name: "get_order_lines"},
		{Section: "workload_tx_delivery", Name: "get_min_new_order"},
		{Section: "workload_tx_delivery", Name: "delete_new_order"},
		{Section: "workload_tx_delivery", Name: "get_order"},
		{Section: "workload_tx_delivery", Name: "update_order"},
		{Section: "workload_tx_delivery", Name: "update_order_line"},
		{Section: "workload_tx_delivery", Name: "get_order_line_amount"},
		{Section: "workload_tx_delivery", Name: "update_customer"},
		{Section: "workload_tx_stock_level", Name: "get_district"},
		{Section: "workload_tx_stock_level", Name: "get_window_items"},
		{Section: "workload_tx_stock_level", Name: "stock_count_in"},
	}
	returningQueries := []workloadtest.Query{
		{Section: "workload_tx_payment", Name: "update_get_warehouse"},
		{Section: "workload_tx_payment", Name: "update_get_district"},
	}
	nonReturningQueries := []workloadtest.Query{
		{Section: "workload_tx_payment", Name: "update_warehouse"},
		{Section: "workload_tx_payment", Name: "get_warehouse"},
		{Section: "workload_tx_payment", Name: "update_district"},
		{Section: "workload_tx_payment", Name: "get_district"},
	}

	for _, dialect := range dialects {
		t.Run(dialect, func(t *testing.T) {
			dialectSections := sections

			dialectQueries := append([]workloadtest.Query(nil), queries...)
			if dialect == "pg.sql" || dialect == "ydb.sql" || dialect == "ydb_no_indexes.sql" {
				dialectQueries = append(dialectQueries, returningQueries...)
			} else {
				dialectQueries = append(dialectQueries, nonReturningQueries...)
			}

			if dialect == "pg.sql" || dialect == "mysql.sql" {
				dialectSections = append(append([]string(nil), sections...), "create_procedures", "workload_procs")
				for _, name := range txNames {
					dialectQueries = append(dialectQueries, workloadtest.Query{Section: "workload_procs", Name: name})
				}
			}

			workloadtest.SQL(t, files, dialect, dialectSections, dialectQueries)
		})
	}
}
