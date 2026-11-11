//go:build integration && integration_optional

package integration

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

const defaultMariaDBURL = "myuser:mypassword@tcp(localhost:3368)/mydb?parseTime=true&multiStatements=true"

func TestTPCDSMariaDBAllQueries(t *testing.T) {
	url := envOr("STROPPY_MARIADB_URL", defaultMariaDBURL)
	newSQLDB(t, "mariadb", "STROPPY_MARIADB_URL", defaultMariaDBURL)

	for _, mode := range []string{"baked", "generated", "throughput"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{
				"run", "tpcds", "-d", "mysql", "--sql-dialect", "mariadb", "-D", "url=" + url,
				"--scale-factor", "0.01", "--load-workers", "4", "--executor", "shared-iterations",
			}
			want := 103

			switch mode {
			case "baked":
				args = append(args, "--no-steps", "validate_answers", "--iterations", "1", "--vus", "1")
			case "generated":
				args = append(args, "--steps", "workload", "--query-stream", "0", "--iterations", "1", "--vus", "1")
			case "throughput":
				args = append(args, "--steps", "workload", "--streams", "2", "--iterations", "2", "--vus", "2")
				want = 206
			}

			output := runStroppy(t, 10*time.Minute, args...)
			if got := strings.Count(output, "query completed"); got != want {
				t.Fatalf("successful statements=%d, want %d", got, want)
			}
		})
	}
}

func TestTPCDSMariaDBRollupMatchesMySQL(t *testing.T) {
	mariaURL := envOr("STROPPY_MARIADB_URL", defaultMariaDBURL)
	maria := newSQLDB(t, "mariadb", "STROPPY_MARIADB_URL", defaultMariaDBURL)

	mysql := NewMySQL(t)
	for _, url := range []string{mariaURL, envOr(envMySQLAllURL, defaultMySQLAllURL)} {
		runStroppy(t, time.Minute, "run", "tpcds", "-d", "mysql", "-D", "url="+url, "--steps", "create_schema")
	}

	fixtures := []string{
		`INSERT INTO date_dim(d_date_sk,d_year,d_moy,d_month_seq) VALUES(1,2000,1,1200),(2,2001,1,1201),(3,2002,1,1202)`,
		`INSERT INTO item(i_item_sk,i_item_id,i_category,i_class,i_brand,i_product_name) VALUES
 (1,'item1','Books','Class','Brand','Product'),(2,'item2','Books',NULL,'Brand','Product'),
 (3,NULL,NULL,NULL,NULL,NULL),(4,'item4','Music','Class','Brand','Product')`,
		`INSERT INTO store(s_store_sk,s_store_id,s_state,s_county) VALUES(1,'store1','TN','County'),(2,'store2','TN',NULL)`,
		`INSERT INTO customer_demographics(cd_demo_sk,cd_gender,cd_marital_status,cd_education_status)
 VALUES(1,'M','S','College')`,
		`INSERT INTO store_sales(ss_item_sk,ss_sold_date_sk,ss_store_sk,ss_cdemo_sk,ss_quantity,ss_list_price,
 ss_coupon_amt,ss_sales_price,ss_net_profit,ss_ext_sales_price) VALUES
 (1,1,1,1,2,10,1,20,2,20),(1,2,1,1,2,10,1,20,2,20),(1,3,1,1,2,10,1,20,2,20),
 (2,2,2,1,3,20,2,30,6,30),(3,2,2,1,4,30,3,40,12,40),(4,2,1,1,5,40,4,50,20,50),
 (3,3,2,1,4,30,3,40,12,40)`,
		`INSERT INTO web_sales(ws_item_sk,ws_sold_date_sk,ws_net_paid) VALUES(1,1,10),(2,1,20),(3,1,30),(4,1,40)`,
	}
	for _, db := range []*sql.DB{maria, mysql} {
		for _, fixture := range fixtures {
			if _, err := db.ExecContext(t.Context(), fixture); err != nil {
				t.Fatal(err)
			}
		}
	}

	repo, _ := stroppyBinary(t)
	load := func(name string) *bench.SQL {
		data, err := os.ReadFile(filepath.Join(repo, "workloads", "tpcds", name))
		if err != nil {
			t.Fatal(err)
		}

		return bench.ParseSQL(string(data))
	}
	mariaSQL, mysqlSQL := load("mariadb.sql"), load("mysql.sql")

	for _, name := range []string{
		"query_5", "query_14_a", "query_18", "query_22", "query_27", "query_36",
		"query_67", "query_70", "query_77", "query_80", "query_86",
	} {
		t.Run(name, func(t *testing.T) {
			left, leftOK := mariaSQL.Query("", name)
			right, rightOK := mysqlSQL.Query("", name)

			if !leftOK || !rightOK {
				t.Fatal("missing rollup query")
			}

			got, want := tpcdsComparableRows(t, maria, left), tpcdsComparableRows(t, mysql, right)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("MariaDB rows=%v\nMySQL rows=%v", got, want)
			}

			switch name {
			case "query_27", "query_36", "query_70", "query_86":
				if len(got) == 0 {
					t.Fatal("grouping-level fixture returned no rows")
				}
			}
		})
	}

	t.Run("ranked_limit", func(t *testing.T) {
		items, sales := make([]string, 0, 120), make([]string, 0, 120)
		for i := range 120 {
			items = append(items, fmt.Sprintf("(%d,'Books','class%d')", i+100, i))
			sales = append(sales, fmt.Sprintf("(%d,1,%d)", i+100, i+100))
		}

		for _, db := range []*sql.DB{maria, mysql} {
			for _, statement := range []string{
				"INSERT INTO item(i_item_sk,i_category,i_class) VALUES " + strings.Join(items, ","),
				"INSERT INTO web_sales(ws_item_sk,ws_sold_date_sk,ws_net_paid) VALUES " + strings.Join(sales, ","),
			} {
				if _, err := db.ExecContext(t.Context(), statement); err != nil {
					t.Fatal(err)
				}
			}
		}

		left, _ := mariaSQL.Query("", "query_86")
		right, _ := mysqlSQL.Query("", "query_86")

		got, want := tpcdsComparableRows(t, maria, left), tpcdsComparableRows(t, mysql, right)
		if len(got) != 100 || !reflect.DeepEqual(got, want) {
			t.Fatalf("limited MariaDB rows=%v\nMySQL rows=%v", got, want)
		}
	})

	// All predicates exclude the fixture: rollup must not invent an empty grand total.
	for _, name := range []string{"query_27", "query_36", "query_70", "query_86"} {
		left, _ := mariaSQL.Query("", name)
		right, _ := mysqlSQL.Query("", name)
		left = strings.ReplaceAll(left, "where", "where 1=0 and")
		right = strings.ReplaceAll(right, "where", "where 1=0 and")

		got, want := tpcdsComparableRows(t, maria, left), tpcdsComparableRows(t, mysql, right)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s empty rollup differs: %v / %v", name, got, want)
		}
	}
}

func tpcdsComparableRows(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()

	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}

	var result []string

	for rows.Next() {
		values := make([]any, len(columns))

		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}

		if err := rows.Scan(targets...); err != nil {
			t.Fatal(err)
		}

		cells := make([]string, len(columns))

		for i, value := range values {
			if value == nil {
				cells[i] = "<NULL>"

				continue
			}

			text := fmt.Sprint(value)
			if bytes, ok := value.([]byte); ok {
				text = string(bytes)
			}

			if number, err := strconv.ParseFloat(text, 64); err == nil {
				text = strconv.FormatFloat(number, 'f', 5, 64)
			}

			cells[i] = text
		}

		result = append(result, strings.Join(cells, "|"))
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	sort.Strings(result)

	return result
}
