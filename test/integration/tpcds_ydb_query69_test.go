//go:build integration && integration_optional

package integration

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestTPCDSYDBQualificationRowsQuery69(t *testing.T) {
	driver := NewYDB(t)

	connector, err := ydbsdk.Connector(driver, ydbsdk.WithQueryService(true))
	if err != nil {
		t.Fatal(err)
	}

	db := sql.OpenDB(connector)
	defer db.Close()

	repo, _ := stroppyBinary(t)

	data, err := os.ReadFile(filepath.Join(repo, "workloads", "tpcds", "ydb.sql"))
	if err != nil {
		t.Fatal(err)
	}

	query, ok := bench.ParseSQL(string(data)).Query("", "query_69")
	if !ok {
		t.Fatal("query 69 missing")
	}

	for _, mode := range []string{"row", "column"} {
		t.Run(mode, func(t *testing.T) {
			url := envOr(envYDBAllURL, defaultYDBAllURL)
			runStroppy(t, 2*time.Minute, "run", "tpcds", "-d", "ydb", "-D", "url="+url,
				"--ydb-store-mode", mode, "--steps", "drop_schema,create_schema")

			for _, fixture := range []string{
				`UPSERT INTO date_dim(d_date_sk,d_year,d_moy) VALUES
 (1,2001,4),(2,2001,5),(3,2001,6),(4,2001,3),(5,2001,7),(6,2000,4);`,
				`UPSERT INTO customer_address(ca_address_sk,ca_state) VALUES(1,"KY"),(2,"TN");`,
				`UPSERT INTO customer_demographics(cd_demo_sk,cd_gender,cd_marital_status,
 cd_education_status,cd_purchase_estimate,cd_credit_rating) VALUES(1,"F","M","College",500,"Good");`,
				`UPSERT INTO customer(c_customer_sk,c_current_addr_sk,c_current_cdemo_sk) VALUES
 (1,1,1),(2,1,1),(3,1,1),(4,1,1),(5,1,1),(6,1,1),(7,1,1),(8,2,1),(9,1,1);`,
				`UPSERT INTO store_sales(ss_item_sk,ss_ticket_number,ss_sold_date_sk,ss_customer_sk) VALUES
 (1,1,1,1),(1,2,2,1),(1,3,3,1),(1,4,1,2),(1,5,1,3),(1,6,1,4),
 (1,7,4,6),(1,8,6,7),(1,9,1,8),(1,10,3,9),(1,11,1,NULL);`,
				`UPSERT INTO web_sales(ws_item_sk,ws_order_number,ws_sold_date_sk,
 ws_bill_customer_sk,ws_ship_customer_sk) VALUES
 (1,1,1,2,9),(1,2,2,2,9),(1,3,2,4,4),(1,4,1,NULL,1),(1,5,4,1,1),(1,6,5,9,9);`,
				`UPSERT INTO catalog_sales(cs_item_sk,cs_order_number,cs_sold_date_sk,
 cs_ship_customer_sk,cs_bill_customer_sk) VALUES
 (1,1,3,3,1),(1,2,2,3,1),(1,3,2,4,4),(1,4,1,NULL,9),(1,5,6,1,1);`,
			} {
				if _, err := db.ExecContext(t.Context(), fixture); err != nil {
					t.Fatal(err)
				}
			}

			rows, err := db.QueryContext(t.Context(), query)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()

			columns, err := rows.Columns()

			wantColumns := []string{
				"cd_gender", "cd_marital_status", "cd_education_status", "cnt1",
				"cd_purchase_estimate", "cnt2", "cd_credit_rating", "cnt3",
			}
			if err != nil || !reflect.DeepEqual(columns, wantColumns) {
				t.Fatalf("columns=%v error=%v, want %v", columns, err, wantColumns)
			}

			if !rows.Next() {
				t.Fatalf("query 69 returned no row: %v", rows.Err())
			}

			var got [8]string
			if err := rows.Scan(&got[0], &got[1], &got[2], &got[3], &got[4], &got[5], &got[6], &got[7]); err != nil {
				t.Fatal(err)
			}

			want := [8]string{"F", "M", "College", "2", "500", "2", "Good", "2"}
			if got != want {
				t.Fatalf("query 69 returned %v, want %v", got, want)
			}

			if rows.Next() || rows.Err() != nil {
				t.Fatalf("query 69 returned extra rows or failed: %v", rows.Err())
			}

			sqlFile := filepath.Join(t.TempDir(), "query69.sql")
			if err := os.WriteFile(sqlFile, []byte("--= query_69\n"+query), 0o600); err != nil {
				t.Fatal(err)
			}

			runStroppy(t, time.Minute, "run", "tpcds", "-d", "ydb", "-D", "url="+url,
				"--sql-file", sqlFile, "--steps", "workload", "--iterations", "1")
		})
	}
}
