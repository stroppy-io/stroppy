package tpcc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestYDBPartitionDefaults(t *testing.T) {
	data, err := files.ReadFile("ydb.sql")
	if err != nil {
		t.Fatal(err)
	}

	sql := bench.ParseSQL(string(data))
	for _, section := range []string{"create_schema", "create_indexes"} {
		for _, query := range sql.Section(section) {
			if strings.Contains(query, "AUTO_PARTITIONING_MAX_PARTITIONS_COUNT") ||
				strings.Contains(query, "PARTITION_SIZE_MB = 500") || strings.Contains(query, "= 1000") {
				t.Fatalf("%s retains deployment-sized limits: %s", section, query)
			}
		}
	}

	for _, table := range []string{"district", "customer", "new_order", "orders", "order_line", "stock"} {
		query, ok := sql.Query("create_schema", table)
		if !ok || !strings.Contains(query, "PARTITION_AT_KEYS = ({partition_keys})") ||
			!strings.Contains(query, "AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = {partition_count}") {
			t.Fatalf("%s lost its warehouse layout", table)
		}
	}

	for _, table := range []string{"item", "history"} {
		query, _ := sql.Query("create_schema", table)
		if strings.Contains(query, "PARTITIONS_COUNT") {
			t.Fatalf("%s retains a fixed partition floor", table)
		}
	}

	for _, name := range []string{"idx_customer_name", "idx_order"} {
		query, _ := sql.Query("create_indexes", name)
		if !strings.Contains(query, "GLOBAL SYNC") {
			t.Fatalf("%s lost synchronous maintenance", name)
		}
	}
}

func TestSetupErrorIdentifiesStatement(t *testing.T) {
	backend := errors.New("Query compilation timed out")
	recorder := &record.Recorder{}
	recorder.Reply("SELECT 2;", record.Response{Err: backend})

	test := bench.Test{Name: "setup-error", Define: func(d *bench.Def) error {
		w := &workload{sql: bench.ParseSQL("--+ create_indexes\n--= first\nSELECT 1;\n--= failing_index\nSELECT 2;")}
		section := sqlSection{workload: w, name: "create_indexes"}

		d.Execution.Step("create_indexes", func(ctx context.Context, b *bench.Bench) error {
			return section.Run(ctx, b)
		})

		return d.Execution.Err()
	}}

	_, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	if !errors.Is(err, backend) || !strings.Contains(err.Error(), `statement 2 ("failing_index")`) ||
		!strings.Contains(err.Error(), "driver recording, elapsed") {
		t.Fatalf("setup error lost identity or cause: %v", err)
	}
}
