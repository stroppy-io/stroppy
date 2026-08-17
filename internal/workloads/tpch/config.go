package tpch

import (
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

const preset = "tpch"

// tpchTables is the load order: parents before children (FK consistency).
var tpchTables = [8]string{"region", "nation", "part", "supplier", "partsupp", "customer", "orders", "lineitem"}

// queryNames is q1..q22, in TPC-H spec order.
var queryNames [22]string

func init() {
	for i := range queryNames {
		queryNames[i] = fmt.Sprintf("q%d", i+1)
	}
}

func sqlFile(dt bench.DriverTypeName, override string) string {
	if override != "" {
		return override
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

func mustLoadSQL(dt bench.DriverTypeName, override string) *bench.SQL {
	s, err := bench.LoadSQL(preset, sqlFile(dt, override))
	if err != nil {
		panic(err)
	}

	return s
}

// shiftDate mirrors tpch tx.ts: shift an ISO date by (days, months, years) in UTC
// and return the new ISO date. picodata/ydb lack date+interval arithmetic, so the
// anchor dates for those dialects are precomputed client-side.
func shiftDate(iso string, days, months, years int) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}

	return t.AddDate(years, months, days).Format("2006-01-02")
}

// queryParams builds the §2.4 pinned parameter set. q11 fraction is SF-dependent.
func queryParams(scaleFactor float64) map[string]map[string]any {
	m := map[string]map[string]any{
		"q1":  {"delta": 90},
		"q2":  {"size": 15, "type": "BRASS", "region": "EUROPE"},
		"q3":  {"segment": "BUILDING", "date": "1995-03-15"},
		"q4":  {"date": "1993-07-01"},
		"q5":  {"region": "ASIA", "date": "1994-01-01"},
		"q6":  {"date": "1994-01-01", "discount": 0.06, "quantity": 24},
		"q7":  {"nation1": "FRANCE", "nation2": "GERMANY"},
		"q8":  {"region": "AMERICA", "nation": "BRAZIL", "type": "ECONOMY ANODIZED STEEL"},
		"q9":  {"color": "green"},
		"q10": {"date": "1993-10-01"},
		"q11": {"nation": "GERMANY", "fraction": 0.0001 / scaleFactor},
		"q12": {"shipmode1": "MAIL", "shipmode2": "SHIP", "date": "1994-01-01"},
		"q13": {"word1": "special", "word2": "requests"},
		"q14": {"date": "1995-09-01"},
		"q15": {"date": "1996-01-01"},
		"q16": {
			"brand": "Brand#45", "type_prefix": "MEDIUM POLISHED",
			"s1": 49, "s2": 14, "s3": 23, "s4": 45, "s5": 19, "s6": 3, "s7": 36, "s8": 9,
		},
		"q17": {"brand": "Brand#23", "container": "MED BOX"},
		"q18": {"quantity": 300},
		"q19": {"brand1": "Brand#12", "brand2": "Brand#23", "brand3": "Brand#34", "q1": 1, "q2": 10, "q3": 20},
		"q20": {"color": "forest", "nation": "CANADA", "date": "1994-01-01"},
		"q21": {"nation": "SAUDI ARABIA"},
		"q22": {"cc1": "13", "cc2": "31", "cc3": "23", "cc4": "29", "cc5": "30", "cc6": "18", "cc7": "17"},
	}

	return m
}

// withEndDates adds date_1m/date_3m/date_1y derived from `date` for picodata/ydb
// (which lack date+interval expressions). No-op on pg/mysql or when no `date` param.
func withEndDates(p map[string]any, needsEndDates bool) map[string]any {
	if !needsEndDates {
		return p
	}

	d, ok := p["date"].(string)
	if !ok {
		return p
	}

	p["date_1m"] = shiftDate(d, 0, 1, 0)
	p["date_3m"] = shiftDate(d, 0, 3, 0)
	p["date_1y"] = shiftDate(d, 0, 0, 1)

	return p
}
