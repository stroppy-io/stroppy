//go:build integration

package integration

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/dsqgen"
)

func TestTPCDSQuery75ZeroNetSales(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	runStroppy(t, time.Minute, "run", "tpcds", "-d", "pg",
		"-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL), "--steps", "create_schema")

	data, err := os.ReadFile("../../workloads/tpcds/pg.sql")
	if err != nil {
		t.Fatal(err)
	}
	baked, ok := bench.ParseSQL(string(data)).Query("", "query_75")
	if !ok {
		t.Fatal("missing baked query 75")
	}
	queries := map[string]string{"baked": baked}
	for _, stream := range []int{0, 5, 6, 7, 8} {
		result, err := dsqgen.Generate(dsqgen.Postgres, 10, 19620718, stream)
		if err != nil {
			t.Fatal(err)
		}
		for _, query := range result.Queries {
			if query.Name == "query75" {
				queries[fmt.Sprintf("stream_%d", stream)] = query.SQL
			}
		}
	}
	if len(queries) != 6 {
		t.Fatal("missing generated query 75")
	}

	yearPattern := regexp.MustCompile(`curr_yr.d_year=(\d+)`)
	categoryPattern := regexp.MustCompile(`i_category='([^']+)'`)
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			year, category := yearPattern.FindStringSubmatch(query), categoryPattern.FindStringSubmatch(query)
			if len(year) != 2 || len(category) != 2 {
				t.Fatal("missing query 75 substitutions")
			}
			_, err := pool.Exec(t.Context(), fmt.Sprintf(`
TRUNCATE date_dim,item,catalog_sales,catalog_returns,store_sales,store_returns,web_sales,web_returns;
INSERT INTO date_dim (d_date_sk,d_year) VALUES (1,`+year[1]+`-1),(2,`+year[1]+`);
INSERT INTO item (i_item_sk,i_brand_id,i_class_id,i_category_id,i_manufact_id,i_category)
VALUES (1,1,1,1,1,'%[1]s'),(2,2,1,1,1,'%[1]s'),(3,3,1,1,1,'%[1]s'),(4,4,1,1,1,'%[1]s');
INSERT INTO catalog_sales (cs_item_sk,cs_order_number,cs_sold_date_sk,cs_quantity,cs_ext_sales_price)
VALUES (1,1,1,10,100),(1,2,2,5,50);
INSERT INTO catalog_returns (cr_item_sk,cr_order_number,cr_return_quantity,cr_return_amount)
VALUES (1,1,10,100);
INSERT INTO store_sales (ss_item_sk,ss_ticket_number,ss_sold_date_sk,ss_quantity,ss_ext_sales_price)
VALUES (2,1,1,10,100),(2,2,2,8,80),(4,3,1,10,100),(4,4,2,5,50);
INSERT INTO store_returns (sr_item_sk,sr_ticket_number,sr_return_quantity,sr_return_amt)
VALUES (4,4,5,50);
INSERT INTO web_sales (ws_item_sk,ws_order_number,ws_sold_date_sk,ws_quantity,ws_ext_sales_price)
VALUES (3,1,1,10,100),(3,2,2,9,90);`, strings.ReplaceAll(category[1], "'", "''")))
			if err != nil {
				t.Fatal(err)
			}

			AssertTableEquals(t, pool,
				"SELECT i_brand_id,prev_yr_cnt,curr_yr_cnt,sales_cnt_diff FROM ("+
					strings.TrimSuffix(strings.TrimSpace(query), ";")+") q75 ORDER BY i_brand_id",
				[]map[string]any{
					{"i_brand_id": int32(2), "prev_yr_cnt": int64(10), "curr_yr_cnt": int64(8), "sales_cnt_diff": int64(-2)},
					{"i_brand_id": int32(4), "prev_yr_cnt": int64(10), "curr_yr_cnt": int64(0), "sales_cnt_diff": int64(-10)},
				})
		})
	}
}
