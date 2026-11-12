//go:build integration && integration_optional

package integration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/options"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestYDBConservativeDefaults(t *testing.T) {
	native := NewYDB(t)

	connector, err := ydbsdk.Connector(native, ydbsdk.WithQueryService(true))
	if err != nil {
		t.Fatal(err)
	}

	db := sql.OpenDB(connector)

	t.Cleanup(func() { _ = db.Close() })

	url := envOr(envYDBAllURL, defaultYDBAllURL)

	t.Run("simple", func(t *testing.T) { testYDBSimpleNative(t, db, url) })
	t.Run("tpcc", func(t *testing.T) { testYDBTPCCTableDefaults(t, native, db, url) })

	for _, workload := range []string{"tpch/tx", "tpcds"} {
		t.Run(workload, func(t *testing.T) { testYDBAnalyticalRowDefaults(t, native, url, workload) })
	}
}

func testYDBSimpleNative(t *testing.T, db *sql.DB, url string) {
	t.Helper()

	output := runStroppy(t, time.Minute, "run", "simple", "-d", "ydb", "-D", "url="+url,
		"--iterations", "1", "--no-steps", "cleanup")
	if !strings.Contains(output, `"method": "native"`) {
		t.Fatalf("YDB load did not use native insertion: %s", output)
	}

	var count int64
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM stroppy_demo").Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 100 {
		t.Fatalf("simple rows=%d, want 100", count)
	}

	if _, err := db.ExecContext(t.Context(), "DROP TABLE stroppy_demo"); err != nil {
		t.Fatal(err)
	}
}

func testYDBTPCCTableDefaults(t *testing.T, native *ydbsdk.Driver, db *sql.DB, url string) {
	t.Helper()

	runStroppy(t, time.Minute, "run", "tpcc/tx", "-d", "ydb", "-D", "url="+url,
		"--scale-factor", "4", "--steps", "drop_schema,create_schema")

	for _, name := range []string{"district", "customer", "new_order", "orders", "order_line", "stock"} {
		desc := describeYDBTable(t, native, name)
		if desc.PartitioningSettings.MinPartitionsCount != 4 || desc.PartitioningSettings.MaxPartitionsCount == 4 ||
			desc.Stats.Partitions != 4 {
			t.Fatalf("%s warehouse layout or growth policy changed: %+v", name, desc)
		}
	}

	for _, name := range []string{"item", "history"} {
		desc := describeYDBTable(t, native, name)
		if desc.PartitioningSettings.MinPartitionsCount != 1 || desc.Stats.Partitions != 1 {
			t.Fatalf("%s retains deployment-sized floor: %+v", name, desc)
		}
	}

	customer := types.ListValue(types.StructValue(
		types.StructFieldValue("c_w_id", types.Int64Value(1)),
		types.StructFieldValue("c_d_id", types.Int64Value(1)),
		types.StructFieldValue("c_id", types.Int64Value(1)),
		types.StructFieldValue("c_last", types.UTF8Value("last")),
		types.StructFieldValue("c_first", types.UTF8Value("first")),
	))
	orders := types.ListValue(types.StructValue(
		types.StructFieldValue("o_w_id", types.Int64Value(1)),
		types.StructFieldValue("o_d_id", types.Int64Value(1)),
		types.StructFieldValue("o_id", types.Int64Value(1)),
		types.StructFieldValue("o_c_id", types.Int64Value(1)),
	))

	for name, rows := range map[string]types.Value{"customer": customer, "orders": orders} {
		err := native.Table().BulkUpsert(t.Context(), native.Name()+"/"+name, table.BulkUpsertDataRows(rows))
		if err != nil {
			t.Fatal(err)
		}
	}

	runStroppy(t, time.Minute, "run", "tpcc/tx", "-d", "ydb", "-D", "url="+url, "--steps", "create_indexes")

	for _, name := range []string{"customer/idx_customer_name/indexImplTable", "orders/idx_order/indexImplTable"} {
		desc := describeYDBTable(t, native, name)
		if desc.PartitioningSettings.MinPartitionsCount != 1 || desc.PartitioningSettings.MaxPartitionsCount == 1000 ||
			desc.PartitioningSettings.PartitionSizeMb == 500 {
			t.Fatalf("%s retains fixed index tuning: %+v", name, desc)
		}
	}

	for _, query := range []string{
		"SELECT c_id FROM customer VIEW idx_customer_name WHERE c_w_id=1 AND c_d_id=1 AND c_last='last'",
		"SELECT o_id FROM orders VIEW idx_order WHERE o_w_id=1 AND o_d_id=1 AND o_c_id=1",
	} {
		var id int64
		if err := db.QueryRowContext(t.Context(), query).Scan(&id); err != nil {
			t.Fatal(err)
		}

		if id != 1 {
			t.Fatalf("index lookup returned %d", id)
		}
	}

	runStroppy(t, time.Minute, "run", "tpcc/tx", "-d", "ydb", "-D", "url="+url, "--steps", "drop_schema")
}

func testYDBAnalyticalRowDefaults(t *testing.T, native *ydbsdk.Driver, url, workload string) {
	t.Helper()

	runStroppy(t, time.Minute, "run", workload, "-d", "ydb", "-D", "url="+url,
		"--ydb-store-mode", "row", "--steps", "drop_schema,create_schema")

	repo, _ := stroppyBinary(t)

	file := filepath.Join(repo, "workloads", "tpch", "ydb.sql")
	if workload == "tpcds" {
		file = filepath.Join(repo, "workloads", "tpcds", "schema.ydb.sql")
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	for _, query := range bench.ParseSQL(string(data)).Section("create_schema") {
		words := strings.Fields(query)

		desc := describeYDBTable(t, native, words[2])
		if desc.PartitioningSettings.MinPartitionsCount != 1 || desc.Stats.Partitions != 1 {
			t.Fatalf("%s row table retains fixed floor: %+v", words[2], desc)
		}
	}

	runStroppy(t, time.Minute, "run", workload, "-d", "ydb", "-D", "url="+url, "--steps", "drop_schema")
}

func describeYDBTable(t *testing.T, native *ydbsdk.Driver, name string) options.Description {
	t.Helper()

	var desc options.Description

	err := native.Table().Do(t.Context(), func(ctx context.Context, session table.Session) error {
		var err error

		desc, err = session.DescribeTable(ctx, native.Name()+"/"+name, options.WithTableStats())

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	return desc
}
