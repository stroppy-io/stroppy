package tpcc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// newOrderTestSQL is a minimal workload_tx_new_order section whose query bodies
// carry unique markers so the fake tx can dispatch each read to a canned row set.
const newOrderTestSQL = `
--+ workload_tx_new_order
--= get_customer
SELECT CUSTOMER_ROW
--= get_warehouse
SELECT WAREHOUSE_ROW
--= get_district
SELECT DISTRICT_ROW
--= update_district
UPDATE_DISTRICT
--= insert_order
INSERT_ORDER_ROW
--= insert_new_order
INSERT_NEW_ORDER
--= get_items_batch
SELECT ITEM_ROWS {item_ids}
--= get_stocks_batch
SELECT STOCK_ROWS {item_ids}
--= update_stock
UPDATE_STOCK_ROW
--= insert_order_line
INSERT_ORDER_LINE
`

// fakeRows is a minimal driver.Rows over a canned row set.
type fakeRows struct {
	rows [][]any
	idx  int
}

func (r *fakeRows) Columns() []string { return nil }
func (r *fakeRows) Next() bool {
	if r.idx < len(r.rows) {
		r.idx++

		return true
	}

	return false
}
func (r *fakeRows) Values() []any         { return r.rows[r.idx-1] }
func (r *fakeRows) ReadAll(_ int) [][]any { return r.rows }
func (r *fakeRows) Err() error            { return nil }
func (r *fakeRows) Close() error          { return nil }

// fakeTx implements driver.Tx; RunQuery dispatches on the SQL markers above.
type fakeTx struct {
	respond     func(sql string) ([][]any, error)
	commitErr   error
	rollbackErr error
}

func (t *fakeTx) RunQuery(_ context.Context, sql string, _ map[string]any) (*driver.QueryResult, error) {
	rows, err := t.respond(sql)
	if err != nil {
		return nil, err
	}

	return &driver.QueryResult{Rows: &fakeRows{rows: rows}}, nil
}

func (t *fakeTx) Commit(context.Context) error       { return t.commitErr }
func (t *fakeTx) Rollback(context.Context) error     { return t.rollbackErr }
func (t *fakeTx) Isolation() config.TxIsolationLevel { return config.TxIsolationLevelReadCommitted }

// fakeDriver implements driver.Driver and hands out a pre-wired tx.
type fakeDriver struct{ tx driver.Tx }

func (d *fakeDriver) Insert(context.Context, *driver.InsertRequest) (*stats.Query, error) {
	return &stats.Query{}, nil
}

func (d *fakeDriver) RunQuery(context.Context, string, map[string]any) (*driver.QueryResult, error) {
	return &driver.QueryResult{}, nil
}

func (d *fakeDriver) Begin(context.Context, config.TxIsolationLevel) (driver.Tx, error) {
	return d.tx, nil
}

func (d *fakeDriver) ClassifyError(error) driver.ErrorFacts { return driver.ErrorFacts{} }
func (d *fakeDriver) Teardown(context.Context) error        { return nil }

const (
	fakeDriverType           = config.DriverType(99)
	newOrderTestWorkloadName = "tpcc/test-new-order"
)

var (
	currentDriver = &fakeDriver{}
	currentRunner = &newOrderRunner{}
)

func init() {
	driver.RegisterDriver(fakeDriverType, func(context.Context, driver.Options) (driver.Driver, error) {
		return currentDriver, nil
	})

	bench.Register(func() bench.Workload { return currentRunner })
}

// newOrderRunner is a thin workload that invokes the new-order body once with
// canned line items, capturing the resulting error.
type newOrderRunner struct {
	w             *workload
	lineIID       []int64
	lineQty       []int64
	lineSupply    []int64
	forceRollback bool
	err           error
}

func (*newOrderRunner) Name() string                                 { return newOrderTestWorkloadName }
func (*newOrderRunner) Define(*bench.Def) error                      { return nil }
func (*newOrderRunner) Setup(context.Context, *bench.Bench) error    { return nil }
func (*newOrderRunner) Teardown(context.Context, *bench.Bench) error { return nil }

func (r *newOrderRunner) Iterate(ctx context.Context, b *bench.Bench) error {
	tx, err := b.Begin(ctx, bench.BeginOpts{Isolation: bench.IsoReadCommitted, Name: "new_order"})
	if err != nil {
		r.err = err

		return err
	}

	r.err = r.w.newOrderBody(ctx, tx,
		1, 1, 1, int64(len(r.lineIID)), 1,
		r.lineIID, r.lineQty, r.lineSupply, r.forceRollback)

	return r.err
}

// runNewOrderBody executes newOrderBody once against the given tx row responses
// and returns the resulting error.
func runNewOrderBody(
	t *testing.T,
	respond func(sql string) ([][]any, error),
	lineIID []int64,
	forceRollback bool,
) error {
	t.Helper()

	w := &workload{sql: bench.ParseSQL(newOrderTestSQL), variant: "tx"}

	lineQty := make([]int64, len(lineIID))
	lineSupply := make([]int64, len(lineIID))

	for i := range lineIID {
		lineQty[i] = 5
		lineSupply[i] = 1
	}

	currentDriver = &fakeDriver{tx: &fakeTx{respond: respond}}
	currentRunner = &newOrderRunner{
		w: w, lineIID: lineIID, lineQty: lineQty, lineSupply: lineSupply,
		forceRollback: forceRollback,
	}

	if err := bench.Run(
		context.Background(),
		newOrderTestWorkloadName,
		map[int]*config.DriverConfig{0: {DriverType: fakeDriverType}},
		nil,
		bench.ParamInputs{},
		zap.NewNop(),
		&bench.MetricsConfig{},
	); err != nil {
		t.Fatalf("bench.Run failed: %v", err)
	}

	return currentRunner.err
}

// newOrderResponds returns a responder keyed by the SQL markers above. For each
// issued query it returns the rows registered for the first marker contained in
// the SQL (or no rows/error for unregistered reads and Exec statements).
func newOrderResponds(m map[string][][]any) func(sql string) ([][]any, error) {
	return func(sql string) ([][]any, error) {
		for marker, rows := range m {
			if strings.Contains(sql, marker) {
				return rows, nil
			}
		}

		return nil, nil
	}
}

func customerRow() []any       { return []any{int64(1)} }
func warehouseRow() []any      { return []any{int64(1)} }
func districtRow() []any       { return []any{int64(2101)} }
func itemRow(iid int64) []any  { return []any{iid, float64(10)} }
func stockRow(iid int64) []any { return []any{iid, int64(50), "", "dist"} }

func TestNewOrderBodyMissingCustomer(t *testing.T) {
	err := runNewOrderBody(t, newOrderResponds(map[string][][]any{
		"WAREHOUSE_ROW": {warehouseRow()},
		"DISTRICT_ROW":  {districtRow()},
	}), []int64{1}, false)
	if !errors.Is(err, errNewOrderCustomerMissing) {
		t.Fatalf("missing customer error = %v, want %v", err, errNewOrderCustomerMissing)
	}
}

func TestNewOrderBodyMissingWarehouse(t *testing.T) {
	err := runNewOrderBody(t, newOrderResponds(map[string][][]any{
		"CUSTOMER_ROW": {customerRow()},
		"DISTRICT_ROW": {districtRow()},
	}), []int64{1}, false)
	if !errors.Is(err, errNewOrderWarehouseMissing) {
		t.Fatalf("missing warehouse error = %v, want %v", err, errNewOrderWarehouseMissing)
	}
}

func TestNewOrderBodyMissingItem(t *testing.T) {
	err := runNewOrderBody(t, newOrderResponds(map[string][][]any{
		"CUSTOMER_ROW":  {customerRow()},
		"WAREHOUSE_ROW": {warehouseRow()},
		"DISTRICT_ROW":  {districtRow()},
		"ITEM_ROWS":     {itemRow(10)}, // item 20 absent
		"STOCK_ROWS":    {stockRow(10)},
	}), []int64{10, 20}, false)
	if !errors.Is(err, errItemNotFound) {
		t.Fatalf("missing item error = %v, want %v", err, errItemNotFound)
	}
}

func TestNewOrderBodyForcedRollbackReportsMissingRegularItem(t *testing.T) {
	err := runNewOrderBody(t, newOrderResponds(map[string][][]any{
		"CUSTOMER_ROW":  {customerRow()},
		"WAREHOUSE_ROW": {warehouseRow()},
		"DISTRICT_ROW":  {districtRow()},
	}), []int64{10, items + 1}, true)
	if !errors.Is(err, errItemNotFound) {
		t.Fatalf("forced rollback with missing regular item error = %v, want %v", err, errItemNotFound)
	}
}

func TestNewOrderBodyMissingStock(t *testing.T) {
	err := runNewOrderBody(t, newOrderResponds(map[string][][]any{
		"CUSTOMER_ROW":  {customerRow()},
		"WAREHOUSE_ROW": {warehouseRow()},
		"DISTRICT_ROW":  {districtRow()},
		"ITEM_ROWS":     {itemRow(10)},
		// STOCK_ROWS omitted: item 10 has no stock row.
	}), []int64{10}, false)
	if !errors.Is(err, errNewOrderStockMissing) {
		t.Fatalf("missing stock error = %v, want %v", err, errNewOrderStockMissing)
	}
}

func TestFinishNewOrderSentinelSuccess(t *testing.T) {
	if err := finishNewOrder(errRollbackSentinel, nil); err != nil {
		t.Fatalf("sentinel + nil rollback = %v, want nil", err)
	}
}

func TestFinishNewOrderSentinelRollbackFailure(t *testing.T) {
	rbErr := errors.New("rollback failed")

	err := finishNewOrder(errRollbackSentinel, rbErr)
	if err == nil {
		t.Fatal("sentinel + rollback error = nil, want error (unknown outcome must not be success)")
	}

	if !errors.Is(err, rbErr) {
		t.Fatalf("error %v does not wrap rollback error %v", err, rbErr)
	}
}

func TestFinishNewOrderPropagatesRollbackError(t *testing.T) {
	rbErr := errors.New("rollback failed")

	err := finishNewOrder(errItemNotFound, rbErr)
	if !errors.Is(err, errItemNotFound) || !errors.Is(err, rbErr) {
		t.Fatalf("error %v should wrap both body %v and rollback %v", err, errItemNotFound, rbErr)
	}
}

func TestFinishNewOrderPlainError(t *testing.T) {
	if err := finishNewOrder(errItemNotFound, nil); !errors.Is(err, errItemNotFound) {
		t.Fatalf("error = %v, want %v", err, errItemNotFound)
	}
}
