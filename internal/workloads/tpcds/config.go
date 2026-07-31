// Package tpcds is the Go-native port of workloads/tpcds/tpcds.ts: the relational load
// of the 24 TPC-DS tables via the ported dsdgen generator (bench.InsertTpcds) plus the
// 99 business queries, run either from the baked canonical qualification set or from an
// in-process generated stream (throughput test). SF=1 answer validation (pg/mysql) is
// ported from tpcds_validate.ts as a multiset comparison against answers_sf1.json.
package tpcds

import "github.com/stroppy-io/stroppy/pkg/bench"

const preset = "tpcds"

// TPCDS_TABLES is the load order: dimensions and static tables first, fan-out fact
// tables last (each returns after its parent sales table). Matches tpcds.ts.
var TPCDS_TABLES = [24]string{
	"income_band", "ship_mode", "reason", "household_demographics",
	"customer_demographics", "date_dim", "time_dim", "warehouse",
	"web_page", "web_site", "catalog_page", "customer_address",
	"customer", "call_center", "store", "promotion", "item", "inventory",
	"store_sales", "store_returns", "catalog_sales", "catalog_returns",
	"web_sales", "web_returns",
}

// dialectFile maps a driver to its (schema, query) SQL files. SQL_FILE / SCHEMA_FILE
// env overrides take precedence. Dialects without their own query file fall back to pg.
func dialectFiles(dt bench.DriverTypeName) (schema, queries string) {
	switch dt {
	case bench.DriverMySQL:
		return bench.Env("SCHEMA_FILE", "schema.mysql.sql"), bench.Env("SQL_FILE", "mysql.sql")
	case bench.DriverPicodata:
		return bench.Env("SCHEMA_FILE", "schema.pico.sql"), bench.Env("SQL_FILE", "pico.sql")
	case bench.DriverYDB:
		return bench.Env("SCHEMA_FILE", "schema.ydb.sql"), bench.Env("SQL_FILE", "ydb.sql")
	default:
		return bench.Env("SCHEMA_FILE", "schema.pg.sql"), bench.Env("SQL_FILE", "pg.sql")
	}
}

func mustLoad(preset, file string) *bench.SQL {
	s, err := bench.LoadSQL(preset, file)
	if err != nil {
		panic(err)
	}

	return s
}
