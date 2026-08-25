//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

const tpchMultiSF = 0.01

type tpchCounts struct {
	region, nation, part, supplier, partsupp, customer, orders, lineitem int64
}

func tpchExpected() tpchCounts {
	scaled := func(base int64) int64 {
		n := int64(math.Floor(float64(base) * tpchMultiSF))
		if n < 1 {
			return 1
		}

		return n
	}

	part := scaled(200_000)
	orders := scaled(1_500_000)

	return tpchCounts{
		region:   5,
		nation:   25,
		part:     part,
		supplier: scaled(10_000),
		partsupp: part * 4,
		customer: scaled(150_000),
		orders:   orders,
		lineitem: 60_175,
	}
}

// TestTpchLoadOnMySQL covers native load and all 22 query bodies on the
// mandatory MySQL service.
func TestTpchLoadOnMySQL(t *testing.T) {
	db := NewMySQL(t)
	ResetMySQL(t, db, tpchTables)

	url := envOr(envMySQLAllURL, defaultMySQLAllURL)
	out := runTpchStroppy(t, "mysql", url, 3*time.Minute)

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsMySQL(t, db)
	assertTpchFKIntegrityMySQL(t, db)
	assertTpchQueriesLogged(t, out)
}

func runTpchStroppy(t *testing.T, driverType, url string, budget time.Duration) string {
	t.Helper()

	start := time.Now()
	scaleFactor := strconv.FormatFloat(tpchMultiSF, 'g', -1, 64)
	out := runStroppy(t, 5*time.Minute,
		"run", "tpch/tx",
		"-D", "url="+url,
		"-D", "driverType="+driverType,
		"--scale-factor", scaleFactor,
		"--load-workers", "4",
		"--executor", "shared-iterations",
		"--iterations", "1",
		"--steps", "drop_schema,create_schema,load_data,create_indexes,analyze,workload",
	)
	elapsed := time.Since(start)
	t.Logf("stroppy TPC-H run on %s completed in %s", driverType, elapsed)

	if elapsed > budget {
		t.Errorf("run on %s took %s, exceeds the %s SF=%s budget", driverType, elapsed, budget, scaleFactor)
	}

	return out
}

func assertTpchLoadMarkers(t *testing.T, out string) {
	t.Helper()

	for _, table := range []string{
		"region", "nation", "part", "supplier", "partsupp", "customer", "orders", "lineitem",
	} {
		marker := fmt.Sprintf(`"event": "completed", "table": %q`, table)
		if !strings.Contains(out, marker) {
			t.Errorf("missing completed insert progress for %q in stroppy output", table)
		}
	}
}

func assertTpchRowCountsMySQL(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	want := tpchExpected()
	checks := []struct {
		table string
		want  int64
	}{
		{"region", want.region},
		{"nation", want.nation},
		{"part", want.part},
		{"supplier", want.supplier},
		{"partsupp", want.partsupp},
		{"customer", want.customer},
		{"orders", want.orders},
		{"lineitem", want.lineitem},
	}
	for _, check := range checks {
		var got int64
		row := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", check.table))
		if err := row.Scan(&got); err != nil {
			t.Fatalf("count(%s): %v", check.table, err)
		}
		if got != check.want {
			t.Errorf("%s: got %d, want %d", check.table, got, check.want)
		}
	}
}

func assertTpchFKIntegrityMySQL(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	checks := []struct {
		name  string
		query string
	}{
		{"supplier.s_nationkey → nation", `SELECT COUNT(*) FROM supplier s
		 WHERE NOT EXISTS (SELECT 1 FROM nation n WHERE n.n_nationkey = s.s_nationkey)`},
		{"customer.c_nationkey → nation", `SELECT COUNT(*) FROM customer c
		 WHERE NOT EXISTS (SELECT 1 FROM nation n WHERE n.n_nationkey = c.c_nationkey)`},
		{"partsupp.ps_partkey → part", `SELECT COUNT(*) FROM partsupp ps
		 WHERE NOT EXISTS (SELECT 1 FROM part p WHERE p.p_partkey = ps.ps_partkey)`},
		{"partsupp.ps_suppkey → supplier", `SELECT COUNT(*) FROM partsupp ps
		 WHERE NOT EXISTS (SELECT 1 FROM supplier s WHERE s.s_suppkey = ps.ps_suppkey)`},
		{"orders.o_custkey → customer", `SELECT COUNT(*) FROM orders o
		 WHERE NOT EXISTS (SELECT 1 FROM customer c WHERE c.c_custkey = o.o_custkey)`},
		{"lineitem.l_orderkey → orders", `SELECT COUNT(*) FROM lineitem l
		 WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.o_orderkey = l.l_orderkey)`},
		{"lineitem.l_partkey → part", `SELECT COUNT(*) FROM lineitem l
			 WHERE NOT EXISTS (SELECT 1 FROM part p WHERE p.p_partkey = l.l_partkey)`},
		{"lineitem.l_suppkey → supplier", `SELECT COUNT(*) FROM lineitem l
			 WHERE NOT EXISTS (SELECT 1 FROM supplier s WHERE s.s_suppkey = l.l_suppkey)`},
	}
	for _, check := range checks {
		var orphans int64
		if err := db.QueryRowContext(ctx, check.query).Scan(&orphans); err != nil {
			t.Fatalf("FK %s: %v", check.name, err)
		}
		if orphans != 0 {
			t.Errorf("FK %s: %d orphan rows", check.name, orphans)
		}
	}
}
