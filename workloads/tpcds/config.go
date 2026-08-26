// Package tpcds owns Stroppy's TPC-DS implementation, tests, dialect SQL, and
// answer data. It loads 24 tables through the canonical generator and runs the
// 99 business queries, run either from the baked canonical qualification set or from an
// in-process generated stream (throughput test). SF=1 answer validation (pg/mysql) is
// ported from tpcds_validate.ts as a multiset comparison against answers_sf1.json.
package tpcds

import "github.com/stroppy-io/stroppy/pkg/bench"

const preset = "tpcds"

// tpcdsTables is the load order: dimensions and static tables first, fan-out fact
// tables last (each returns after its parent sales table). Matches tpcds.ts.
var tpcdsTables = [24]string{
	"income_band", "ship_mode", "reason", "household_demographics",
	"customer_demographics", "date_dim", "time_dim", "warehouse",
	"web_page", "web_site", "catalog_page", "customer_address",
	"customer", "call_center", "store", "promotion", "item", "inventory",
	"store_sales", "store_returns", "catalog_sales", "catalog_returns",
	"web_sales", "web_returns",
}

// dialectFiles maps a driver to its (schema, query) SQL files. Explicit file
// parameters take precedence over driver defaults.
func dialectFiles(dt bench.DriverTypeName, schemaOverride, queryOverride string) (schema, queries string) {
	switch dt {
	case bench.DriverMySQL:
		schema, queries = "schema.mysql.sql", "mysql.sql"
	case bench.DriverPicodata:
		schema, queries = "schema.pico.sql", "pico.sql"
	case bench.DriverYDB:
		schema, queries = "schema.ydb.sql", "ydb.sql"
	default:
		schema, queries = "schema.pg.sql", "pg.sql"
	}

	if schemaOverride != "" {
		schema = schemaOverride
	}

	if queryOverride != "" {
		queries = queryOverride
	}

	return schema, queries
}

func mustLoad(preset, file string) *bench.SQL {
	s, err := bench.LoadSQL(preset, file)
	if err != nil {
		panic(err)
	}

	return s
}
