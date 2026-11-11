//go:build integration

package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/dsqgen"
)

func TestTPCDSGeneratedPostgresAllQueries(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	reportFile := filepath.Join(t.TempDir(), "report.json")
	out := runStroppy(t, 5*time.Minute,
		"run", "tpcds", "-d", "pg", "-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL),
		"--scale-factor", "0.01", "--query-stream", "0", "--query-seed", "19620718",
		"--executor", "shared-iterations", "--iterations", "1", "--vus", "1",
		"--report-file", reportFile,
	)

	matches := regexp.MustCompile(`query completed\s+\{[^\n]*"query": "query_(\d+)(?:_[abc])?"`).
		FindAllStringSubmatch(out, -1)
	if len(matches) != 103 {
		t.Fatalf("got %d successful SQL statements, want 103\n%s", len(matches), out)
	}
	seen := make(map[string]bool)
	for _, match := range matches {
		seen[match[1]] = true
	}
	for i := 1; i <= 99; i++ {
		if !seen[strconv.Itoa(i)] {
			t.Errorf("Q%d missing", i)
		}
	}
	raw, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Steps []struct {
			Name       string
			Executions int
		}
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, step := range envelope.Steps {
		if step.Name == "workload" {
			if step.Executions != 1 {
				t.Fatalf("preflight counted as a workload execution: %d", step.Executions)
			}
			return
		}
	}
	t.Fatal("workload step missing from report")
}

func TestTPCDSGeneratedMySQLAllQueries(t *testing.T) {
	db := NewMySQL(t)
	url := envOr(envMySQLAllURL, defaultMySQLAllURL)
	testTPCDSFullJoinResults(t, db, "mysql", url)
	runStroppy(t, 10*time.Minute,
		"run", "tpcds", "-d", "mysql", "-D", "url="+url,
		"--scale-factor", "0.01", "--load-workers", "4",
		"--steps", "create_schema,load_data,create_indexes,analyze",
	)
	testTPCDSGeneratedMySQLDialect(t, "mysql", url)
}

func testTPCDSFullJoinResults(t *testing.T, db *sql.DB, driver, url string) {
	t.Helper()
	runStroppy(t, time.Minute,
		"run", "tpcds", "-d", driver, "-D", "url="+url, "--steps", "create_schema",
	)

	testTPCDSCarrierResults(t, db)

	result, err := dsqgen.Generate(dsqgen.MySQL, 0.01, 19620718, 0)
	if err != nil {
		t.Fatal(err)
	}

	monthPattern := regexp.MustCompile(`d_month_seq between (\d+)`)

	for _, query := range result.Queries {
		if query.Name != "query51" && query.Name != "query97" {
			continue
		}

		t.Run(query.Name+"/results", func(t *testing.T) {
			for _, table := range []string{"web_sales", "store_sales", "catalog_sales", "date_dim"} {
				if _, err := db.ExecContext(t.Context(), "DELETE FROM "+table); err != nil {
					t.Fatal(err)
				}
			}

			month := monthPattern.FindStringSubmatch(query.SQL)
			if len(month) != 2 {
				t.Fatal("generated month substitution missing")
			}

			statements := []string{
				"INSERT INTO date_dim (d_date_sk,d_date,d_month_seq) VALUES " +
					"(1,'2000-01-01'," + month[1] + "),(2,'2000-01-02'," + month[1] + "),(3,'2000-01-03'," + month[1] + ")",
				"INSERT INTO web_sales (ws_item_sk,ws_sold_date_sk,ws_sales_price) VALUES " +
					"(1,1,20),(1,1,10),(1,3,10),(2,1,50),(3,1,5)",
				"INSERT INTO store_sales (ss_customer_sk,ss_item_sk,ss_sold_date_sk,ss_sales_price) VALUES " +
					"(1,1,1,10),(1,1,1,5),(1,1,2,1),(2,4,1,10),(NULL,6,1,1),(3,NULL,1,1),(4,7,1,1)",
				"INSERT INTO catalog_sales (cs_bill_customer_sk,cs_item_sk,cs_sold_date_sk) VALUES " +
					"(1,1,1),(1,1,1),(5,5,1),(NULL,6,1),(3,NULL,1),(4,8,1)",
			}
			for _, statement := range statements {
				if _, err := db.ExecContext(t.Context(), statement); err != nil {
					t.Fatal(err)
				}
			}

			rows, err := db.QueryContext(t.Context(), query.SQL)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()

			if query.Name == "query97" {
				assertTPCDSQ97(t, rows)
			} else {
				assertTPCDSQ51(t, rows)
			}

			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func testTPCDSCarrierResults(t *testing.T, db *sql.DB) {
	t.Helper()

	carrierExpression := regexp.MustCompile(`(?i)(concat\([^\n]+\)) as ship_carriers`)
	quotedString := regexp.MustCompile(`'((?:[^']|'')*)'`)

	for _, seed := range []int64{1, 42, 19620718} {
		result, err := dsqgen.Generate(dsqgen.MySQL, 0.01, seed, 0)
		if err != nil {
			t.Fatal(err)
		}

		var expression string

		for _, query := range result.Queries {
			if query.Name == "query66" {
				match := carrierExpression.FindStringSubmatch(query.SQL)
				if len(match) == 2 {
					expression = match[1]
				}
			}
		}

		parts := quotedString.FindAllStringSubmatch(expression, -1)
		if len(parts) != 3 {
			t.Fatalf("seed %d: missing generated carrier concatenation", seed)
		}

		want := strings.ReplaceAll(parts[0][1]+parts[1][1]+parts[2][1], "''", "'")

		var got string
		if err := db.QueryRowContext(t.Context(), "SELECT "+expression).Scan(&got); err != nil {
			t.Fatal(err)
		}

		if got != want {
			t.Errorf("seed %d: ship_carriers = %q, want %q", seed, got, want)
		}
	}
}

func assertTPCDSQ97(t *testing.T, rows *sql.Rows) {
	t.Helper()

	var storeOnly, catalogOnly, both int

	if !rows.Next() {
		t.Fatalf("missing counts: %v", rows.Err())
	}

	if err := rows.Scan(&storeOnly, &catalogOnly, &both); err != nil {
		t.Fatal(err)
	}

	if storeOnly != 3 || catalogOnly != 3 || both != 1 {
		t.Fatalf("counts = %d,%d,%d; want 3,3,1", storeOnly, catalogOnly, both)
	}
}

func assertTPCDSQ51(t *testing.T, rows *sql.Rows) {
	t.Helper()

	var got []string

	for rows.Next() {
		var (
			item                                 int
			date, webCumulative, storeCumulative string
			web, store                           sql.NullString
		)
		if err := rows.Scan(&item, &date, &web, &store, &webCumulative, &storeCumulative); err != nil {
			t.Fatal(err)
		}

		got = append(got, fmt.Sprintf("%d/%s/%s/%s/%s/%s",
			item, date[:10], web.String, store.String, webCumulative, storeCumulative))
	}

	want := "1/2000-01-01/30.00/15.00/30.00/15.00," +
		"1/2000-01-02//16.00/30.00/16.00,1/2000-01-03/40.00//40.00/16.00"
	if strings.Join(got, ",") != want {
		t.Fatalf("rows = %v; want %s", got, want)
	}
}

func testTPCDSGeneratedMySQLDialect(t *testing.T, driver, url string) {
	t.Helper()

	for _, mode := range []string{"power", "throughput"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{
				"run", "tpcds", "-d", driver, "-D", "url=" + url,
				"--scale-factor", "0.01", "--load-workers", "4", "--query-seed", "19620718",
				"--executor", "shared-iterations", "--steps", "workload",
			}
			want := 103

			if mode == "power" {
				args = append(args, "--query-stream", "0", "--iterations", "1", "--vus", "1")
			} else {
				args = append(args, "--streams", "2", "--iterations", "2", "--vus", "2")
				want *= 2
			}

			out := runStroppy(t, 10*time.Minute, args...)

			matches := regexp.MustCompile(`query completed\s+\{[^\n]*"query": "query_(\d+)(?:_[abc])?"`).
				FindAllStringSubmatch(out, -1)
			if len(matches) != want {
				t.Fatalf("got %d successful SQL statements, want %d\n%s", len(matches), want, out)
			}

			seen := make(map[string]int)
			for _, match := range matches {
				seen[match[1]]++
			}

			for i := 1; i <= 99; i++ {
				if seen[strconv.Itoa(i)] == 0 {
					t.Errorf("Q%d missing", i)
				}
			}
		})
	}
}
