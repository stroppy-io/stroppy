//go:build integration && integration_optional

package integration

import (
	"database/sql"
	"encoding/json"
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
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/dsqgen"
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
				args = append(args, "--steps", "workload", "--streams", "2", "--vus", "2",
					"--executor", "constant-vus", "--duration", "1s", "--drain-timeout", "none", "--log-mode", "production")
				want = 206
			}

			output := runStroppy(t, 10*time.Minute, args...)
			if got := strings.Count(output, "query completed"); got != want {
				t.Fatalf("successful statements=%d, want %d", got, want)
			}

			if mode == "throughput" {
				completed := make(map[int]int)

				for _, line := range strings.Split(output, "\n") {
					if !strings.Contains(line, "query completed") {
						continue
					}

					var entry struct {
						Worker int `json:"worker"`
					}

					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						t.Fatal(err)
					}

					completed[entry.Worker]++
				}

				if completed[0] != 103 || completed[1] != 103 {
					t.Fatalf("per-worker statement counts=%v, want 103 each", completed)
				}
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
		`INSERT INTO date_dim(d_date_sk,d_year,d_moy,d_month_seq,d_date) VALUES
 (1,2000,8,1200,'2000-08-23'),(2,2001,1,1201,'2001-01-01'),(3,2002,1,1202,'2002-01-01'),
 (4,2001,11,1203,'2001-11-01'),(5,1998,1,1204,'1998-01-01')`,
		`INSERT INTO item(i_item_sk,i_item_id,i_category,i_class,i_brand,i_product_name,
 i_brand_id,i_class_id,i_category_id,i_current_price) VALUES
 (1,'item1','Books','Class','Brand','Product',1,1,1,60),(2,'item2','Books',NULL,'Brand','Product',1,1,1,60),
 (3,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,60),(4,'item4','Music','Class','Brand','Product',2,2,2,60)`,
		`INSERT INTO store(s_store_sk,s_store_id,s_state,s_county) VALUES(1,'store1','TN','County'),(2,'store2','TN',NULL)`,
		`INSERT INTO customer_demographics(cd_demo_sk,cd_gender,cd_marital_status,cd_education_status)
 VALUES(1,'M','S','College')`,
		`INSERT INTO store_sales(ss_item_sk,ss_sold_date_sk,ss_store_sk,ss_cdemo_sk,ss_quantity,ss_list_price,
 ss_coupon_amt,ss_sales_price,ss_net_profit,ss_ext_sales_price) VALUES
 (1,1,1,1,2,10,1,20,2,20),(1,2,1,1,2,10,1,20,2,20),(1,3,1,1,2,10,1,20,2,20),
 (2,2,2,1,3,20,2,30,6,30),(3,2,2,1,4,30,3,40,12,40),(4,2,1,1,5,40,4,50,20,50),
 (3,3,2,1,4,30,3,40,12,40)`,
		`INSERT INTO web_sales(ws_item_sk,ws_sold_date_sk,ws_net_paid) VALUES(1,1,10),(2,1,25),(3,1,30),(4,1,40)`,
		`INSERT INTO inventory(inv_date_sk,inv_item_sk,inv_warehouse_sk,inv_quantity_on_hand) VALUES(1,1,1,10)`,
		`INSERT INTO store_sales(ss_item_sk,ss_sold_date_sk,ss_store_sk,ss_quantity,ss_list_price)
 VALUES(1,4,1,10,20)`,
		`INSERT INTO catalog_sales(cs_item_sk,cs_sold_date_sk,cs_quantity,cs_list_price,cs_bill_cdemo_sk,
 cs_bill_customer_sk) VALUES(1,4,10,20,1,1),(1,5,2,10,2,1)`,
		`INSERT INTO web_sales(ws_item_sk,ws_sold_date_sk,ws_quantity,ws_list_price) VALUES(1,4,10,20)`,
		`INSERT INTO customer_demographics(cd_demo_sk,cd_gender,cd_education_status) VALUES(2,'F','Unknown')`,
		`INSERT INTO customer(c_customer_sk,c_current_cdemo_sk,c_current_addr_sk,c_birth_month,c_birth_year)
 VALUES(1,2,1,1,1970)`,
		`INSERT INTO customer_address(ca_address_sk,ca_country,ca_state,ca_county) VALUES(1,'US','MS','County')`,
		`INSERT INTO promotion(p_promo_sk,p_channel_tv) VALUES(1,'N')`,
		`UPDATE store_sales SET ss_promo_sk=1`,
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
			if name != "query_86" {
				sort.Strings(got)
				sort.Strings(want)
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("MariaDB rows=%v\nMySQL rows=%v", got, want)
			}

			if len(got) == 0 {
				t.Fatal("rollup fixture returned no rows")
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

func TestTPCDSMariaDBGeneratedDateArithmetic(t *testing.T) {
	db := newSQLDB(t, "mariadb", "STROPPY_MARIADB_URL", defaultMariaDBURL)

	result, err := dsqgen.Generate(dsqgen.MariaDB, 0.01, 19620718, 0)
	if err != nil {
		t.Fatal(err)
	}

	var predicate string

	for _, query := range result.Queries {
		if query.Name != "query72" {
			continue
		}

		for _, line := range strings.Split(query.SQL, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "and d3.d_date >") {
				predicate = strings.TrimPrefix(line, "and ")
			}
		}
	}

	if predicate == "" {
		t.Fatal("generated query72 has no shipping-date predicate")
	}

	for _, test := range []struct {
		ship string
		want bool
	}{
		{"2001-02-01", false}, {"2001-02-05", false}, {"2001-02-06", true},
	} {
		var got bool

		err := db.QueryRowContext(t.Context(), "SELECT "+predicate+
			" FROM (SELECT CAST('2001-01-31' AS DATE) d_date) d1"+
			" CROSS JOIN (SELECT CAST(? AS DATE) d_date) d3", test.ship).Scan(&got)
		if err != nil {
			t.Fatal(err)
		}

		if got != test.want {
			t.Fatalf("ship=%s: generated predicate=%t, want %t", test.ship, got, test.want)
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

	return result
}
