//go:build integration && integration_optional

package integration

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestTPCDSYDBQualificationRows(t *testing.T) {
	driver := NewYDB(t)
	url := envOr(envYDBAllURL, defaultYDBAllURL)
	runStroppy(t, 2*time.Minute, "run", "tpcds", "-d", "ydb", "-D", "url="+url,
		"--steps", "drop_schema,create_schema")

	connector, err := ydbsdk.Connector(driver, ydbsdk.WithQueryService(true))
	if err != nil {
		t.Fatal(err)
	}

	db := sql.OpenDB(connector)
	defer db.Close()

	fixtures := []string{
		`UPSERT INTO date_dim(d_date_sk,d_year,d_moy,d_qoy,d_dow) VALUES
 (1,1999,1,1,1),(2,1999,2,1,1),(3,1999,3,1,1),(4,2000,1,1,1),(5,2001,1,1,1),(6,2002,1,1,1);`,
		`UPSERT INTO item(i_item_sk,i_item_id,i_category,i_brand) VALUES(1,"item1","Books","Brand");`,
		`UPSERT INTO store(s_store_sk,s_store_name,s_company_name,s_city,s_number_employees) VALUES
 (1,"ese","company","city",250);`,
		`UPSERT INTO call_center(cc_call_center_sk,cc_name) VALUES(1,"center");`,
		`UPSERT INTO customer_address(ca_address_sk,ca_city,ca_state) VALUES(1,"Edgewood","TN");`,
		`UPSERT INTO customer_demographics(cd_demo_sk,cd_gender,cd_marital_status,cd_dep_count,
 cd_dep_employed_count,cd_dep_college_count) VALUES(1,"M","M",2,3,1);`,
		`UPSERT INTO customer(c_customer_sk,c_customer_id,c_first_name,c_last_name,c_current_addr_sk,
 c_current_cdemo_sk,c_current_hdemo_sk) VALUES(1,"customer1","Rodney","Carter",1,1,1);`,
		`UPSERT INTO household_demographics(hd_demo_sk,hd_dep_count,hd_vehicle_count,hd_income_band_sk) VALUES
 (1,1,2,1),(2,1,3,1),(3,2,4,1);`,
		`UPSERT INTO income_band(ib_income_band_sk,ib_lower_bound,ib_upper_bound) VALUES(1,40000,50000);`,
		`UPSERT INTO time_dim(t_time_sk,t_time,t_hour,t_minute) VALUES(1,40000,11,6),(2,31000,8,35);`,
		`UPSERT INTO warehouse(w_warehouse_sk,w_warehouse_name,w_warehouse_sq_ft,w_city,w_county,w_state,w_country)
 VALUES(1,"warehouse",100,"city","county","TN","United States");`,
		`UPSERT INTO ship_mode(sm_ship_mode_sk,sm_carrier) VALUES(1,"DHL");`,
		`UPSERT INTO store_sales(ss_item_sk,ss_ticket_number,ss_sold_date_sk,ss_store_sk,ss_customer_sk,
 ss_sales_price,ss_quantity,ss_wholesale_cost,ss_hdemo_sk,ss_coupon_amt,ss_net_profit,ss_sold_time_sk) VALUES
 (1,1,1,1,1,10.0,1,1.0,1,1.0,1.0,1),(1,2,2,1,1,100.0,1,1.0,1,1.0,1.0,1),
 (1,3,3,1,1,10.0,1,1.0,1,1.0,1.0,1),(1,4,4,1,1,30.0,6,12.0,1,1.0,1.0,1),
 (1,6,6,1,1,10.0,1,1.0,1,1.0,1.0,1),
 (79,7901,1,1,1,1.0,1,1.0,1,1.0,1.0,1),(79,7902,1,1,1,1.0,1,1.0,2,1.0,1.0,1),
 (88,8801,1,1,1,1.0,1,1.0,3,1.0,1.0,2);`,
		`UPSERT INTO web_sales(ws_item_sk,ws_order_number,ws_sold_date_sk,ws_bill_customer_sk,ws_quantity,
 ws_wholesale_cost,ws_sales_price) VALUES(1,1,4,1,2,4.0,10.0),(1,2,6,1,1,1.0,10.0);`,
		`UPSERT INTO catalog_sales(cs_item_sk,cs_order_number,cs_sold_date_sk,cs_call_center_sk,cs_sales_price,
 cs_quantity,cs_net_paid,cs_net_paid_inc_tax,cs_warehouse_sk,cs_ship_mode_sk,cs_sold_time_sk) VALUES
 (1,1,1,1,10.0,1,10.0,12.0,1,1,1),(1,2,2,1,100.0,1,100.0,120.0,1,1,1),
 (1,3,3,1,10.0,1,10.0,12.0,1,1,1),(1,4,5,1,10.0,2,10.0,12.0,1,1,1);`,
		`UPSERT INTO store_returns(sr_item_sk,sr_ticket_number,sr_cdemo_sk) VALUES(84,84,1);`,
	}
	for _, fixture := range fixtures {
		if _, err := db.ExecContext(t.Context(), fixture); err != nil {
			t.Fatal(err)
		}
	}

	repo, _ := stroppyBinary(t)

	data, err := os.ReadFile(filepath.Join(repo, "workloads", "tpcds", "ydb.sql"))
	if err != nil {
		t.Fatal(err)
	}

	queries := bench.ParseSQL(string(data))

	for _, test := range []struct {
		name    string
		columns int
		want    map[int]string
	}{
		{"query_35", 18, map[int]string{6: "2", 11: "3", 16: "1"}},
		{"query_47", 10, map[int]string{0: "Books", 1: "Brand", 4: "1999", 5: "2", 7: "100"}},
		{"query_57", 9, map[int]string{0: "Books", 3: "1999", 4: "2", 6: "100"}},
		{"query_66", 44, map[int]string{8: "20", 32: "24"}},
		{"query_78", 10, map[int]string{0: "2000", 1: "1", 2: "1", 3: "3"}},
		{"query_84", 2, map[int]string{0: "customer1", 1: "Carter                        , Rodney"}},
		{"query_88", 8, map[int]string{0: "1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, ok := queries.Query("", test.name)
			if !ok {
				t.Fatal("query missing")
			}

			assertTPCDSYDBRow(t, db, query, test.columns, test.want)
		})
	}

	q79, _ := queries.Query("", "query_79")

	rows, err := db.QueryContext(t.Context(), q79)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			last, first, city      any
			ticket, amount, profit any
		)

		if err := rows.Scan(&last, &first, &city, &ticket, &amount, &profit); err != nil {
			t.Fatal(err)
		}

		if fmt.Sprint(ticket) == "7901" {
			t.Fatal("query79 included household below qualification vehicle threshold")
		}
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func assertTPCDSYDBRow(t *testing.T, db *sql.DB, query string, columnCount int, want map[int]string) {
	t.Helper()

	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil || len(columns) != columnCount {
		t.Fatalf("columns=%v error=%v, want %d", columns, err, columnCount)
	}

	matched := false

	for rows.Next() {
		values := make([]any, len(columns))

		targets := make([]any, len(columns))

		for i := range values {
			targets[i] = &values[i]
		}

		if err := rows.Scan(targets...); err != nil {
			t.Fatal(err)
		}

		matches := true

		for index, expected := range want {
			got := fmt.Sprint(values[index])
			if value, isBytes := values[index].([]byte); isBytes {
				got = string(value)
			}

			if got != expected {
				var actualNumber, expectedNumber float64

				_, actualError := fmt.Sscan(got, &actualNumber)
				_, expectedError := fmt.Sscan(expected, &expectedNumber)
				matches = matches && actualError == nil && expectedError == nil &&
					math.Abs(actualNumber-expectedNumber) < 0.0001
			}
		}

		matched = matched || matches
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if !matched {
		t.Fatalf("no fixture row matched qualification values %v", want)
	}
}
