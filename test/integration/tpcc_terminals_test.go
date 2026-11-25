//go:build integration

package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/workloads"
	_ "github.com/stroppy-io/stroppy/v6/workloads/tpcc"
	"go.uber.org/zap"
)

func TestTpccStrictTerminalModel(t *testing.T) {
	for _, backend := range preparedBackends() {
		t.Run(backend.name, func(t *testing.T) {
			db, err := driver.Dispatch(t.Context(), driver.Options{Config: &config.DriverConfig{DriverType: backend.kind, URL: backend.url}, Logger: zap.NewNop()})
			require.NoError(t, err)
			defer db.Teardown(t.Context())
			asset := "pg.sql"
			preset := "pg"
			if backend.kind == config.DriverTypeMySQL {
				asset = "mysql.sql"
				preset = "mysql"
			}
			source, err := workloads.ReadPresetFile("tpcc", asset)
			require.NoError(t, err)
			sql := filepath.Join(t.TempDir(), "delivery.sql")
			// Only Delivery touches the fixture; other terminal transactions keep their timing and selection.
			text := string(source) + `
--+ workload_procs
--= new_order
SELECT 1
--= payment
SELECT 1
--= order_status
SELECT 1
--= delivery_results
`
			if preset == "pg" {
				text += "SELECT DELIVERY_RESULTS(:d_w_id, :d_o_carrier_id)::TEXT\n"
			} else {
				text += "CALL DELIVERY_RESULTS_atomic(:d_w_id, :d_o_carrier_id)\n"
			}
			text += `--= stock_level
SELECT 1
`
			require.NoError(t, os.WriteFile(sql, []byte(text), 0600))
			url := backend.url
			runStroppy(t, time.Minute, "run", "tpcc/procs", sql, "-d", preset, "-D", "url="+url,
				"--steps", "drop_schema,create_schema,create_procedures,create_terminal_procedures", "--terminal-model", "strict", "--executor", "constant-vus")
			queries := []string{"INSERT INTO history(h_id) VALUES (1)"}
			for warehouse := 1; warehouse <= 10; warehouse++ {
				queries = append(queries,
					fmt.Sprintf("INSERT INTO customer(c_w_id,c_d_id,c_id,c_balance,c_delivery_cnt) VALUES (%d,1,1,0,0)", warehouse),
					fmt.Sprintf("INSERT INTO orders(o_w_id,o_d_id,o_id,o_c_id) VALUES (%d,1,2101,1)", warehouse),
					fmt.Sprintf("INSERT INTO new_order(no_w_id,no_d_id,no_o_id) VALUES (%d,1,2101)", warehouse),
					fmt.Sprintf("INSERT INTO order_line(ol_w_id,ol_d_id,ol_o_id,ol_number,ol_amount) VALUES (%d,1,2101,1,10)", warehouse))
			}
			for _, query := range queries {
				result, queryErr := db.RunQuery(t.Context(), query, nil)
				require.NoError(t, queryErr)
				if result != nil && result.Rows != nil {
					require.NoError(t, result.Rows.Close())
				}
			}
			file := filepath.Join(t.TempDir(), "report.json")
			results := filepath.Join(t.TempDir(), "deliveries.jsonl")
			runStroppy(t, time.Minute, "run", "tpcc/procs", sql, "-d", preset, "-D", "url="+url,
				"--steps", "initialize_terminals,workload,delivery_results", "--terminal-model", "strict", "--executor", "constant-vus",
				"--vus", "2", "--scale-factor", "10", "--duration", "4s", "--drain-timeout", "5s",
				"--delivery-results", results, "--report-file", file)
			raw, err := os.ReadFile(file)
			require.NoError(t, err)
			var run report.Run
			require.NoError(t, json.Unmarshal(raw, &run))
			require.Equal(t, report.StatusCompleted, run.Status)
			output, err := os.Open(results)
			require.NoError(t, err)
			defer output.Close()
			scanner := bufio.NewScanner(output)
			count := 0
			delivered := 0
			for scanner.Scan() {
				var result struct {
					Queued, Completed  time.Time
					Warehouse, Carrier int64
					Orders             []struct{ District, Order int64 }
					Error              string
				}
				require.NoError(t, json.Unmarshal(scanner.Bytes(), &result))
				require.Empty(t, result.Error)
				require.False(t, result.Completed.Before(result.Queued))
				for _, order := range result.Orders {
					require.EqualValues(t, 1, order.District)
					require.EqualValues(t, 2101, order.Order)
					delivered++
				}
				count++
			}
			require.NoError(t, scanner.Err())
			require.Positive(t, count)
			require.Positive(t, delivered)
			response, queryErr := db.RunQuery(t.Context(), "SELECT SUM(c_balance), SUM(c_delivery_cnt) FROM customer", nil)
			require.NoError(t, queryErr)
			require.True(t, response.Rows.Next())
			values := response.Rows.Values()
			require.Equal(t, fmt.Sprint(delivered), strings.TrimSuffix(fmt.Sprint(values[1]), ".00"))
			require.NoError(t, response.Rows.Close())
			require.NoError(t, response.Rows.Err())
		})
	}
}
