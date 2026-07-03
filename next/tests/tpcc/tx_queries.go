package main

import (
	"log"

	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// txQueries holds every workload query the client-side transactions use,
// resolved once from the SQL corpus at plan phase. Each is prepared per VU in
// the workload handler's Init (warming vu.Prepare's cache) and then used on the
// hot path as a cache hit.
type txQueries struct {
	// new_order (per-line single-row item/stock reads; no IN-list batch form)
	noGetCustomer, noGetWarehouse, noGetDistrict, noUpdDistrict,
	noInsOrder, noInsNewOrder, noGetItem, noGetStock, noUpdStock, noInsOrderLine *sqlfile.Query

	// payment (RETURNING-merged warehouse/district updates)
	pUpdWh, pUpdDist, pCountByName, pGetByName, pGetByID, pUpdCust, pUpdCustBC, pInsHist *sqlfile.Query

	// order_status
	osGetByID, osCountByName, osGetByName, osGetLastOrder, osGetOrderLines *sqlfile.Query

	// delivery
	dGetMinNO, dDelNO, dGetOrder, dUpdOrder, dUpdOrderLine, dGetAmount, dUpdCust *sqlfile.Query

	// stock_level (district next-o-id + the single-shot count from tpcc.sql)
	slGetDistrict, slCountLow *sqlfile.Query
}

// all returns every resolved query so Init can warm-prepare them uniformly.
func (q *txQueries) all() []*sqlfile.Query {
	return []*sqlfile.Query{
		q.noGetCustomer, q.noGetWarehouse, q.noGetDistrict, q.noUpdDistrict,
		q.noInsOrder, q.noInsNewOrder, q.noGetItem, q.noGetStock, q.noUpdStock, q.noInsOrderLine,
		q.pUpdWh, q.pUpdDist, q.pCountByName, q.pGetByName, q.pGetByID, q.pUpdCust, q.pUpdCustBC, q.pInsHist,
		q.osGetByID, q.osCountByName, q.osGetByName, q.osGetLastOrder, q.osGetOrderLines,
		q.dGetMinNO, q.dDelNO, q.dGetOrder, q.dUpdOrder, q.dUpdOrderLine, q.dGetAmount, q.dUpdCust,
		q.slGetDistrict, q.slCountLow,
	}
}

// resolveTxQueries looks up every workload query from the pg.sql corpus (file)
// and the tpcc.sql supplement (extra), fatally reporting any missing one.
func resolveTxQueries(file, extra *sqlfile.File) *txQueries {
	get := func(f *sqlfile.File, section, name string) *sqlfile.Query {
		q, ok := f.Query(section, name)
		if !ok {
			log.Fatalf("tpcc: missing query %s/%s", section, name)
		}
		return q
	}
	no := "workload_tx_new_order"
	pay := "workload_tx_payment"
	os := "workload_tx_order_status"
	del := "workload_tx_delivery"
	sl := "workload_tx_stock_level"
	return &txQueries{
		noGetCustomer:  get(file, no, "get_customer"),
		noGetWarehouse: get(file, no, "get_warehouse"),
		noGetDistrict:  get(file, no, "get_district"),
		noUpdDistrict:  get(file, no, "update_district"),
		noInsOrder:     get(file, no, "insert_order"),
		noInsNewOrder:  get(file, no, "insert_new_order"),
		noGetItem:      get(file, no, "get_item"),
		noGetStock:     get(file, no, "get_stock"),
		noUpdStock:     get(file, no, "update_stock"),
		noInsOrderLine: get(file, no, "insert_order_line"),

		pUpdWh:       get(file, pay, "update_get_warehouse"),
		pUpdDist:     get(file, pay, "update_get_district"),
		pCountByName: get(file, pay, "count_customers_by_name"),
		pGetByName:   get(file, pay, "get_customer_by_name"),
		pGetByID:     get(file, pay, "get_customer_by_id"),
		pUpdCust:     get(file, pay, "update_customer"),
		pUpdCustBC:   get(file, pay, "update_customer_bc"),
		pInsHist:     get(file, pay, "insert_history"),

		osGetByID:       get(file, os, "get_customer_by_id"),
		osCountByName:   get(file, os, "count_customers_by_name"),
		osGetByName:     get(file, os, "get_customer_by_name"),
		osGetLastOrder:  get(file, os, "get_last_order"),
		osGetOrderLines: get(file, os, "get_order_lines"),

		dGetMinNO:     get(file, del, "get_min_new_order"),
		dDelNO:        get(file, del, "delete_new_order"),
		dGetOrder:     get(file, del, "get_order"),
		dUpdOrder:     get(file, del, "update_order"),
		dUpdOrderLine: get(file, del, "update_order_line"),
		dGetAmount:    get(file, del, "get_order_line_amount"),
		dUpdCust:      get(file, del, "update_customer"),

		slGetDistrict: get(file, sl, "get_district"),
		slCountLow:    get(extra, sl, "count_low_stock"),
	}
}
