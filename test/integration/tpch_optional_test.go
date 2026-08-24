//go:build integration && integration_optional

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"
)

// TestTpchLoadOnPicodata exercises the optional Picodata dialect end to end.
func TestTpchLoadOnPicodata(t *testing.T) {
	pool := NewPicodata(t)
	ResetPico(t, pool, tpchTables)

	url := envOr(envPicoAllURL, defaultPicoAllURL)
	out := runTpchStroppy(t, "picodata", url, 3*time.Minute)

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsPG(t, pool)
	assertTpchFKIntegrityPico(t, pool)
	assertTpchQueriesLogged(t, out)
}

// TestTpchLoadOnYDB exercises the optional YDB dialect end to end.
func TestTpchLoadOnYDB(t *testing.T) {
	drv := NewYDB(t)
	ResetYDB(t, drv, tpchTables)

	url := envOr(envYDBAllURL, defaultYDBAllURL)
	out := runTpchStroppy(t, "ydb", url, 3*time.Minute)

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsYDB(t, drv)
	assertTpchQueriesLogged(t, out)
}

func assertTpchRowCountsPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()
	want := tpchExpected()
	checks := []struct {
		table string
		want  int64
		tol   float64
	}{
		{"region", want.region, 0},
		{"nation", want.nation, 0},
		{"part", want.part, 0.05},
		{"supplier", want.supplier, 0.05},
		{"partsupp", want.partsupp, 0.05},
		{"customer", want.customer, 0.05},
		{"orders", want.orders, 0.05},
		{"lineitem", want.lineitem, 0.20},
	}
	for _, check := range checks {
		var got int64
		if err := pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", check.table)).Scan(&got); err != nil {
			t.Fatalf("count(%s): %v", check.table, err)
		}
		if !withinTol(got, check.want, check.tol) {
			t.Errorf("%s: got %d, want %d ±%.0f%%", check.table, got, check.want, check.tol*100)
		}
	}
}

func assertTpchRowCountsYDB(t *testing.T, drv *ydbsdk.Driver) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	connector, err := ydbsdk.Connector(drv, ydbsdk.WithQueryService(true))
	if err != nil {
		t.Fatalf("ydb connector: %v", err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()

	want := tpchExpected()
	checks := []struct {
		table string
		want  int64
		tol   float64
	}{
		{"region", want.region, 0},
		{"nation", want.nation, 0},
		{"part", want.part, 0.05},
		{"supplier", want.supplier, 0.05},
		{"partsupp", want.partsupp, 0.05},
		{"customer", want.customer, 0.05},
		{"orders", want.orders, 0.05},
		{"lineitem", want.lineitem, 0.20},
	}
	for _, check := range checks {
		var got int64
		row := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) AS n FROM %s", check.table))
		if err := row.Scan(&got); err != nil {
			t.Fatalf("ydb count(%s): %v", check.table, err)
		}
		if !withinTol(got, check.want, check.tol) {
			t.Errorf("ydb %s: got %d, want %d ±%.0f%%", check.table, got, check.want, check.tol*100)
		}
	}
}

// Picodata cannot use correlated NOT EXISTS for this integrity walk, so the
// checks use left joins that its planner supports.
func assertTpchFKIntegrityPico(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()
	checks := []struct {
		name  string
		query string
	}{
		{"supplier.s_nationkey → nation", `SELECT COUNT(*) FROM supplier
		 LEFT JOIN nation ON nation.n_nationkey = supplier.s_nationkey
		 WHERE nation.n_nationkey IS NULL`},
		{"customer.c_nationkey → nation", `SELECT COUNT(*) FROM customer
		 LEFT JOIN nation ON nation.n_nationkey = customer.c_nationkey
		 WHERE nation.n_nationkey IS NULL`},
		{"partsupp.ps_partkey → part", `SELECT COUNT(*) FROM partsupp
		 LEFT JOIN part ON part.p_partkey = partsupp.ps_partkey
		 WHERE part.p_partkey IS NULL`},
		{"partsupp.ps_suppkey → supplier", `SELECT COUNT(*) FROM partsupp
		 LEFT JOIN supplier ON supplier.s_suppkey = partsupp.ps_suppkey
		 WHERE supplier.s_suppkey IS NULL`},
		{"orders.o_custkey → customer", `SELECT COUNT(*) FROM orders
		 LEFT JOIN customer ON customer.c_custkey = orders.o_custkey
		 WHERE customer.c_custkey IS NULL`},
	}
	for _, check := range checks {
		var orphans int64
		if err := pool.QueryRow(ctx, check.query).Scan(&orphans); err != nil {
			t.Fatalf("FK %s: %v", check.name, err)
		}
		if orphans != 0 {
			t.Errorf("FK %s: %d orphan rows", check.name, orphans)
		}
	}
}
