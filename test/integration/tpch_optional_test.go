//go:build integration && integration_optional

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	out := runTpchStroppy(t, "picodata", url, 3*time.Minute,
		"--pico-motion-row-max", "10000000", "--pico-vdbe-opcode-max", "10000000000")

	if strings.Contains(out, "extra arguments provided") {
		t.Error("TPC-H supplied unused Picodata query arguments")
	}

	checksConfig := pool.Config()
	checksConfig.ConnConfig.RuntimeParams["options"] = "sql_vdbe_opcode_max=10000000000,sql_motion_row_max=10000000"
	checks, err := pgxpool.NewWithConfig(t.Context(), checksConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer checks.Close()

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsPG(t, checks)
	assertTpchFKIntegrityPico(t, checks)
	assertTpchQueriesLogged(t, out)
}

func TestTpchPicodataMotionLimit(t *testing.T) {
	pool := NewPicodata(t)
	const query = "SELECT * FROM (SELECT 1 UNION ALL SELECT 2) AS rows"

	_, err := pool.Exec(t.Context(), query+" OPTION (sql_motion_row_max = 1, sql_vdbe_opcode_max = 10000000000)")
	if err == nil || !strings.Contains(err.Error(), "Exceeded maximum number of rows (1) in virtual table") {
		t.Fatalf("motion limit error = %v", err)
	}

	rows, err := pool.Query(t.Context(), query+" OPTION (sql_motion_row_max = 2, sql_vdbe_opcode_max = 10000000000)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	if rows.Err() != nil || count != 2 {
		t.Fatalf("rows = %d, error = %v", count, rows.Err())
	}
}

func TestTpchPicodataSQLOverride(t *testing.T) {
	NewPicodata(t)
	path := filepath.Join(t.TempDir(), "queries.sql")
	const sql = `--+ q1
--= body
SELECT :delta AS configured_delta OPTION (sql_vdbe_opcode_max = 10000000000);
--+ q3
--= body
SELECT :date_1m AS one_month, :date_3m AS three_months, :date_1y AS one_year;
--+ q4
--= body
SELECT * FROM (SELECT 1 UNION ALL SELECT 2) AS rows
OPTION (sql_motion_row_max = 1, sql_vdbe_opcode_max = 10000000000);
--+ q5
--= body
SELECT 'OPTION (not_an_option)' AS text;
--+ q6
--= body
SELECT 1 OPTION (sql_vdbe_opcode_max = 42 /* cap (opcodes), reserved */);
--+ q7
--= body
SELECT 1 OPTION (sql_vdbe_opcode_max = 42 -- opcode cap (max), reserved
);
`
	if err := os.WriteFile(path, []byte(sql), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runStroppy(t, time.Minute, "run", "tpch/tx", "-d", "pico",
		"-D", "url="+envOr(envPicoAllURL, defaultPicoAllURL),
		"--sql-file", path, "--steps", "workload", "--iterations", "1",
		"--pico-motion-row-max", "2")
	for _, query := range []string{"q1", "q3", "q4", "q5", "q6", "q7"} {
		if !strings.Contains(out, `"query": "`+query+`"`) {
			t.Errorf("override query %s did not complete", query)
		}
	}

	if strings.Contains(out, "extra arguments provided") {
		t.Error("override supplied unused arguments")
	}
}

// TestTpchLoadOnYDB exercises the optional YDB dialect end to end.
func TestTpchLoadOnYDB(t *testing.T) {
	drv := NewYDB(t)
	ResetYDB(t, drv, tpchTables)

	url := envOr(envYDBAllURL, defaultYDBAllURL)
	out := runTpchStroppy(t, "ydb", url, 3*time.Minute)

	if strings.Contains(out, "extra arguments provided") {
		t.Error("TPC-H supplied unused YDB query arguments")
	}

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsYDB(t, drv)
	assertTpchQueriesLogged(t, out)
}

// TestTpchLoadOnCockroachDB exercises the optional CockroachDB dialect, which
// speaks the postgres wire but needs its own asset: CRDB rejects the bare ANALYZE
// that pg.sql ends with, and selects crdb.sql instead.
func TestTpchLoadOnCockroachDB(t *testing.T) {
	pool := NewCRDB(t)

	url := envOr(envCRDBAllURL, defaultCRDBAllURL)
	out := runTpchStroppy(t, "crdb", url, 3*time.Minute)

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsPG(t, pool)
	assertTpchFKIntegrityPico(t, pool)
	assertTpchQueriesLogged(t, out)
}

// TestTpchLoadOnYugabyte exercises the optional YugabyteDB dialect. It speaks the
// postgres wire and needs no asset of its own — YSQL accepts the bare ANALYZE that
// CRDB rejects.
func TestTpchLoadOnYugabyte(t *testing.T) {
	pool := NewYugabyte(t)

	url := envOr(envYugabyteAllURL, defaultYugabyteAllURL)
	out := runTpchStroppy(t, "yugabyte", url, 3*time.Minute)

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsPG(t, pool)
	assertTpchFKIntegrityPico(t, pool)
	assertTpchQueriesLogged(t, out)
}

// TestTpchLoadOnTiDB exercises the optional TiDB dialect, which speaks the mysql
// wire and needs no asset of its own.
func TestTpchLoadOnTiDB(t *testing.T) {
	db := NewTiDB(t)
	ResetMySQL(t, db, tpchTables)

	url := envOr(envTiDBAllURL, defaultTiDBAllURL)
	out := runTpchStroppy(t, "tidb", url, 3*time.Minute)

	assertTpchLoadMarkers(t, out)
	assertTpchRowCountsMySQL(t, db)
	assertTpchFKIntegrityMySQL(t, db)
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

func withinTol(got, want int64, tolerance float64) bool {
	if tolerance == 0 {
		return got == want
	}

	difference := float64(got - want)
	if difference < 0 {
		difference = -difference
	}

	return difference <= float64(want)*tolerance+1
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
