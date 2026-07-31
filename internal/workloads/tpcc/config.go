package tpcc

import (
	"github.com/stroppy-io/stroppy/pkg/bench"
)

const preset = "tpcc"

// Hardcoded TPC-C population constants (tpcc_common.ts).
const (
	districtsPerWarehouse = 10
	customersPerDistrict  = 3000
	items                 = 100000
	ordersDelivered       = 2100
	ordersUndelivered     = customersPerDistrict - ordersDelivered // 900
	olCntFixed            = 10
	itemsPerWh            = items // 100000

	customersPerWh = customersPerDistrict * districtsPerWarehouse // 30000
)

// Load-time seeds (0xC0FFEE0n); per-row PRNG derived from (seed, attr, stream, row).
const (
	seedWarehouse     uint64 = 0xC0FFEE01
	seedDistrict      uint64 = 0xC0FFEE02
	seedCustomer      uint64 = 0xC0FFEE03
	seedItem          uint64 = 0xC0FFEE04
	seedStock         uint64 = 0xC0FFEE05
	seedOrders        uint64 = 0xC0FFEE06
	seedOrderLine     uint64 = 0xC0FFEE07
	seedNewOrder      uint64 = 0xC0FFEE08
	ordersPermuteSalt uint64 = 0x1BEEF02CACE1DAD1
)

// Pacing (§5.2.5): keying time is a fixed pre-tx delay; think time is a
// negative-exponential post-tx delay truncated at 10× the mean.
var (
	keyingTime = map[string]int64{ // seconds
		"new_order": 18, "payment": 3, "order_status": 2, "delivery": 2, "stock_level": 2,
	}
	thinkTimeMean = map[string]float64{ // seconds
		"new_order": 12, "payment": 12, "order_status": 10, "delivery": 5, "stock_level": 5,
	}
)

// txMix is the §5.2.3 weighted dispatch: new_order/payment/order_status/delivery/stock_level.
var txWeights = []int{45, 43, 4, 4, 4}

// resolveIsolation mirrors tpcc_common: pg/mysql→repeatable_read, pico→none, ydb→serializable.
func resolveIsolation(dt bench.DriverTypeName) bench.TxIsolationName {
	if v := bench.Env("TX_ISOLATION", ""); v != "" {
		return bench.TxIsolationName(v)
	}

	switch dt {
	case bench.DriverPicodata:
		return bench.IsoNone
	case bench.DriverYDB:
		return bench.IsoSerializable
	default: // postgres, mysql
		return bench.IsoRepeatableRead
	}
}

func sqlFile(dt bench.DriverTypeName) string {
	if v := bench.Env("SQL_FILE", ""); v != "" {
		return v
	}

	switch dt {
	case bench.DriverMySQL:
		return "mysql.sql"
	case bench.DriverPicodata:
		return "pico.sql"
	case bench.DriverYDB:
		return "ydb.sql"
	default:
		return "pg.sql"
	}
}

func mustLoadSQL(dt bench.DriverTypeName) *bench.SQL {
	s, err := bench.LoadSQL(preset, sqlFile(dt))
	if err != nil {
		panic(err)
	}

	return s
}
