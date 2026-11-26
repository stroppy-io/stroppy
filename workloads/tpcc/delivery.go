package tpcc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

type deliveredOrder struct {
	District int64 `json:"district"`
	Order    int64 `json:"order"`
}

type deliveryResult struct {
	Queued           time.Time        `json:"queued"`
	Completed        time.Time        `json:"completed"`
	Warehouse        int64            `json:"warehouse"`
	Carrier          int64            `json:"carrier"`
	Orders           []deliveredOrder `json:"orders"`
	SkippedDistricts []int64          `json:"skipped_districts"`
	Error            string           `json:"error,omitempty"`
}

type deliveryLog struct {
	mu     sync.Mutex
	file   *os.File
	buffer *bufio.Writer
}

func newDeliveryLog(path string) (*deliveryLog, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("tpcc delivery results: %w", err)
	}

	return &deliveryLog{file: file, buffer: bufio.NewWriter(file)}, nil
}

func (l *deliveryLog) write(result *deliveryResult) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return json.NewEncoder(l.buffer).Encode(result)
}

func (l *deliveryLog) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	err := l.buffer.Flush()
	err = errors.Join(err, l.file.Sync(), l.file.Close())
	l.file = nil

	return err
}

func (w *workload) finishDelivery(ctx context.Context, b *bench.Bench, result *deliveryResult) error {
	w.m.deliveryTotal.Add(ctx, 1)

	err := b.LogicalOperation(func() error {
		var err error
		if w.variant == "procs" {
			result.Orders, err = w.procDeliveredOrders(ctx, b, result.Warehouse, result.Carrier)
		} else {
			result.Orders, err = w.deliverOrders(ctx, b, result.Warehouse, result.Carrier)
		}

		return err
	})

	result.Completed = time.Now()
	if err != nil {
		result.Orders = nil
		result.Error = err.Error()
	} else {
		for district := int64(1); district <= districtsPerWarehouse; district++ {
			found := false

			for _, order := range result.Orders {
				if order.District == district {
					found = true

					break
				}
			}

			if !found {
				result.SkippedDistricts = append(result.SkippedDistricts, district)
			}
		}
	}

	m := w.strict
	m.mu.Lock()
	m.stats.delivery(result, m.measurement, m.end, err)
	m.mu.Unlock()

	if writeErr := m.results.write(result); writeErr != nil {
		return bench.Fatal(errors.Join(err, fmt.Errorf("tpcc delivery results: %w", writeErr)))
	}

	return err
}

func (w *workload) procDeliveredOrders(
	ctx context.Context, b *bench.Bench, warehouse, carrier int64,
) ([]deliveredOrder, error) {
	var rows [][]any

	err := b.Retry(ctx, w.retryOptions(ctx), func() error {
		var err error

		rows, err = b.RoutineRows(ctx, bench.BeginOpts{Isolation: w.iso, Name: "delivery"},
			w.q("workload_procs", "delivery_results"), map[string]any{"d_w_id": warehouse, "d_o_carrier_id": carrier})

		return err
	})
	if err != nil {
		return nil, err
	}

	if len(rows) != 1 || len(rows[0]) != 1 {
		return nil, bench.Fatal(fmt.Errorf("%w: Delivery routine must return one JSON result", errStrictTerminals))
	}

	var pairs [][]int64
	if err := json.Unmarshal([]byte(toStr(rows[0][0])), &pairs); err != nil {
		return nil, bench.Fatal(err)
	}

	if pairs == nil {
		return nil, bench.Fatal(fmt.Errorf("%w: Delivery routine must return a JSON array", errStrictTerminals))
	}

	orders := make([]deliveredOrder, 0, len(pairs))
	seen := map[int64]bool{}

	for _, pair := range pairs {
		if len(pair) != 2 {
			return nil, bench.Fatal(fmt.Errorf("%w: invalid Delivery result pair", errStrictTerminals))
		}

		district, order := pair[0], pair[1]
		if district < 1 || district > 10 || order < 1 || seen[district] {
			return nil, bench.Fatal(fmt.Errorf("%w: invalid Delivery routine result", errStrictTerminals))
		}

		seen[district] = true
		orders = append(orders, deliveredOrder{District: district, Order: order})
	}

	return orders, nil
}
