//go:build integration && integration_optional

package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/picodata"
	"github.com/stroppy-io/stroppy/v6/workloads"
	_ "github.com/stroppy-io/stroppy/v6/workloads/tpcc"
)

func TestTpccPicodataAtomicOrderAllocation(t *testing.T) {
	pool := NewPicodata(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	data, err := workloads.ReadPresetFile("tpcc", "pico.sql")
	if err != nil {
		t.Fatal(err)
	}

	sql := bench.ParseSQL(string(data))

	allocation, ok := sql.Query("workload_tx_new_order", "allocate_order_id")
	if !ok {
		t.Fatal("missing allocation query")
	}

	if _, err = pool.Exec(ctx, "DROP TABLE IF EXISTS district"); err != nil {
		t.Fatal(err)
	}

	ddl, ok := sql.Query("create_schema", "district")
	if !ok {
		t.Fatal("missing district schema")
	}

	if _, err = pool.Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, cleanupErr := pool.Exec(context.Background(), "DROP TABLE IF EXISTS district")
		if cleanupErr != nil {
			t.Error(cleanupErr)
		}
	})

	if _, err = pool.Exec(ctx,
		"INSERT INTO district (d_w_id, d_id, d_next_o_id, d_tax) VALUES (1, 1, 3001, 0.1)"); err != nil {
		t.Fatal(err)
	}

	db, err := picodata.NewDriver(ctx, driver.Options{
		Config: &config.DriverConfig{URL: envOr(envPicoAllURL, defaultPicoAllURL)},
		Logger: zap.NewNop(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Teardown(context.Background())

	const workers, allocations = 16, 16

	ids := make(chan int64, workers*allocations)
	failures := make(chan error, workers)

	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			for range allocations {
				id, queryErr := reservePicodataOrderID(ctx, db, allocation)
				if queryErr != nil {
					failures <- queryErr

					return
				}

				ids <- id
			}
		})
	}

	group.Wait()
	close(ids)
	close(failures)

	for queryErr := range failures {
		t.Error(queryErr)
	}

	seen := make(map[int64]bool)
	for id := range ids {
		if id < 3001 || id >= 3001+workers*allocations {
			t.Errorf("order ID %d outside reserved range [3001, %d)", id, 3001+workers*allocations)
		}
		if seen[id] {
			t.Errorf("duplicate order ID: %d", id)
		}

		seen[id] = true
	}

	if len(seen) != workers*allocations {
		t.Fatalf("reserved %d IDs, want %d", len(seen), workers*allocations)
	}

	var next int64
	if err = pool.QueryRow(ctx, "SELECT d_next_o_id FROM district WHERE d_w_id = 1 AND d_id = 1").Scan(&next); err != nil {
		t.Fatal(err)
	}

	if next != 3001+workers*allocations {
		t.Fatalf("next order ID=%d, want %d", next, 3001+workers*allocations)
	}
}

func reservePicodataOrderID(ctx context.Context, db driver.Driver, query string) (int64, error) {
	result, err := db.RunQuery(ctx, query, map[string]any{"w_id": int64(1), "d_id": int64(1)})
	if err != nil {
		return 0, err
	}

	rows := result.Rows
	defer rows.Close()

	if !rows.Next() {
		return 0, errors.Join(errors.New("allocation returned no row"), rows.Err())
	}

	id, valid := rows.Values()[0].(int64)
	if !valid {
		return 0, fmt.Errorf("unexpected allocation ID type: %T", rows.Values()[0])
	}

	for rows.Next() {
	}

	return id, rows.Err()
}
