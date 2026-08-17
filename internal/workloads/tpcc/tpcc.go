// Package tpcc is the Go-native port of workloads/tpcc/tx.ts: the five TPC-C
// transactions as ordered DML steps inside driver transactions, with the standard
// 45/43/4/4/4 mix, full population, and §1.3.1 validation. Load/config/prepare are
// shared structure ported from tpcc_common.ts. Covers pg + mysql; picodata (no
// OFFSET) and ydb (bound IN-list) dialect branches are ported faithfully so all
// four drivers work, but validation is exercised on postgres.
package tpcc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

var txNames = []string{"new_order", "payment", "order_status", "delivery", "stock_level"}

var (
	errProcsDriverUnsupported = errors.New("tpcc/procs only supports postgres and mysql; use tpcc/tx for picodata/ydb")
	errDistrictNotFound       = errors.New("new_order: district not found")
	errItemNotFound           = errors.New("tpcc_rollback:item_not_found")
	errPaymentNoCustomers     = errors.New("payment: no customers match c_last")
	errPaymentByNameNoRow     = errors.New("payment: by-name SELECT returned no row")
	errPaymentCustomerMissing = errors.New("payment: customer not found")
	errPaymentWarehouseMiss   = errors.New("payment: warehouse not found")
	errPaymentDistrictMiss    = errors.New("payment: district not found")
)

type workload struct {
	sql     *bench.SQL
	variant string // "tx" (DML steps) or "procs" (stored procedures)

	driverType   bench.DriverTypeName
	iso          bench.TxIsolationName
	isPicodata   bool
	isYdb        bool
	hasReturning bool
	pacing       bool

	warehouses     int64
	warehouseStart int64
	wIDMax         int64
	loadItems      bool

	m *metrics

	vuStates sync.Map // uint64 -> *vuState
}

type metrics struct {
	newOrderTotal, paymentTotal, orderStatusTotal, deliveryTotal, stockLevelTotal *bench.Metric
	rollbackDecided, rollbackDone                                                 *bench.Metric
	paymentRemote, paymentByname, paymentBc                                       *bench.Metric
	orderStatusByname                                                             *bench.Metric
	remoteLineTotal, remoteLineRemote                                             *bench.Metric
	retryAttempts                                                                 *bench.Metric
	newOrderDur, paymentDur, orderStatusDur, deliveryDur, stockLevelDur           *bench.Metric
}

func init() {
	bench.Register(&workload{variant: "tx"})
	bench.Register(&workload{variant: "procs"})
}

func (w *workload) Name() string { return "tpcc/" + w.variant }

// renderDDL expands the ydb.sql {partition_keys}/{partition_count} tablet-split
// placeholders from the warehouse range (one tablet per warehouse in
// [warehouseStart, wIDMax]). No-op on dialects whose .sql lacks the tokens.
// Ports tpcc_common.ts renderDDL/ydbPartitionKeys.
func (w *workload) renderDDL(s string) string {
	if !strings.Contains(s, "{partition_") {
		return s
	}

	var keys string
	if w.warehouses <= 1 {
		keys = "(" + strconv.FormatInt(w.warehouseStart+1, 10) + ")"
	} else {
		parts := make([]string, 0, w.warehouses)
		for i := w.warehouseStart + 1; i <= w.wIDMax; i++ {
			parts = append(parts, "("+strconv.FormatInt(i, 10)+")")
		}

		keys = strings.Join(parts, ", ")
	}

	count := strconv.FormatInt(max(w.warehouses, 1), 10)
	s = strings.ReplaceAll(s, "{partition_keys}", keys)

	return strings.ReplaceAll(s, "{partition_count}", count)
}

func (w *workload) Setup(ctx context.Context, b *bench.Bench) error {
	w.driverType = b.DriverTypeName()
	if w.variant == "procs" && (w.driverType == bench.DriverPicodata || w.driverType == bench.DriverYDB) {
		return errProcsDriverUnsupported
	}

	w.initConfig()
	useUnlogged := bench.Env("PG_UNLOGGED", "false") == "true" && w.driverType == bench.DriverPostgres
	w.m = w.initMetrics(b)

	loadDays := time.Now().UTC().Unix() / 86400

	runSection := func(name string) error {
		for _, q := range w.sql.Section(name) {
			if err := b.Exec(ctx, q, nil); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}

		return nil
	}

	type step struct {
		name string
		fn   func() error
	}

	var steps []step

	addStep := func(name string, fn func() error) { steps = append(steps, step{name, fn}) }

	addStep("drop_schema", func() error { return runSection("drop_schema") })
	addStep("create_schema", func() error {
		// ydb.sql DDL carries {partition_keys}/{partition_count} tablet-split
		// placeholders that tx.ts renders from the warehouse range; expand them
		// here. No-op on dialects whose .sql lacks the tokens.
		for _, q := range w.sql.Section("create_schema") {
			if err := b.Exec(ctx, w.renderDDL(q), nil); err != nil {
				return fmt.Errorf("create_schema: %w", err)
			}
		}

		return nil
	})

	if w.variant == "procs" {
		addStep("create_procedures", func() error { return runSection("create_procedures") })
	}

	if useUnlogged {
		addStep("set_unlogged", func() error { return runSection("set_unlogged") })
	}

	addStep("load_data", func() error { return w.loadData(ctx, b, loadDays) })
	addStep("create_indexes", func() error { return runSection("create_indexes") })

	if useUnlogged {
		addStep("set_logged", func() error { return runSection("set_logged") })
	}

	addStep("create_foreign_keys", func() error { return runSection("create_foreign_keys") })
	addStep("analyze", func() error { return runSection("analyze") })
	addStep("validate_population", func() error {
		return validatePopulation(ctx, b, w.warehouses, w.warehouseStart, w.wIDMax)
	})

	for _, s := range steps {
		if err := b.Step(s.name, s.fn); err != nil {
			return err
		}
	}

	b.StepBegin("workload")

	return nil
}

// initConfig reads env-driven workload tuning into w fields.
func (w *workload) initConfig() {
	w.warehouses = max(int64(bench.EnvInt("SCALE_FACTOR", bench.EnvInt("WAREHOUSES", 1))), 1)
	w.warehouseStart = int64(bench.EnvInt("WAREHOUSE_START", 1))
	w.wIDMax = w.warehouseStart + w.warehouses - 1

	defLoadItems := w.warehouseStart == 1
	w.loadItems = bench.Env("LOAD_ITEMS", boolStr(defLoadItems)) == "true"

	w.pacing = bench.Env("PACING", "false") == "true"
	w.iso = resolveIsolation(w.driverType)
	w.sql = mustLoadSQL(w.driverType)
	w.isPicodata = w.driverType == bench.DriverPicodata
	w.isYdb = w.driverType == bench.DriverYDB
	w.hasReturning = w.driverType == bench.DriverPostgres || w.driverType == bench.DriverYDB
}

// initMetrics wires the per-transaction counters and duration trends.
func (w *workload) initMetrics(b *bench.Bench) *metrics {
	return &metrics{
		newOrderTotal:     b.Counter("tpcc_new_order_total"),
		paymentTotal:      b.Counter("tpcc_payment_total"),
		orderStatusTotal:  b.Counter("tpcc_order_status_total"),
		deliveryTotal:     b.Counter("tpcc_delivery_total"),
		stockLevelTotal:   b.Counter("tpcc_stock_level_total"),
		rollbackDecided:   b.Counter("tpcc_rollback_decided"),
		rollbackDone:      b.Counter("tpcc_rollback_done"),
		paymentRemote:     b.Counter("tpcc_payment_remote"),
		paymentByname:     b.Counter("tpcc_payment_byname"),
		paymentBc:         b.Counter("tpcc_payment_bc"),
		orderStatusByname: b.Counter("tpcc_order_status_byname"),
		remoteLineTotal:   b.Counter("tpcc_remote_line_total"),
		remoteLineRemote:  b.Counter("tpcc_remote_line_remote"),
		retryAttempts:     b.Counter("tpcc_retry_attempts"),
		newOrderDur:       b.Trend("tpcc_new_order_duration"),
		paymentDur:        b.Trend("tpcc_payment_duration"),
		orderStatusDur:    b.Trend("tpcc_order_status_duration"),
		deliveryDur:       b.Trend("tpcc_delivery_duration"),
		stockLevelDur:     b.Trend("tpcc_stock_level_duration"),
	}
}

// loadData loads warehouse, district, customer, item, stock, orders,
// order_line, and new_order through typed insert requests.
func (w *workload) loadData(ctx context.Context, b *bench.Bench, loadDays int64) error {
	if _, err := b.Insert(ctx, warehouseRequest(w.warehouses, w.warehouseStart)); err != nil {
		return err
	}

	if _, err := b.Insert(ctx, districtRequest(w.warehouses, w.warehouseStart)); err != nil {
		return err
	}

	if _, err := b.Insert(ctx, customerRequest(w.warehouses, w.warehouseStart, loadDays)); err != nil {
		return err
	}

	if w.loadItems {
		if _, err := b.Insert(ctx, itemRequest()); err != nil {
			return err
		}
	}

	if _, err := b.Insert(ctx, stockRequest(w.warehouses, w.warehouseStart)); err != nil {
		return err
	}

	if _, err := b.Insert(ctx, ordersRequest(w.warehouses, w.warehouseStart, loadDays)); err != nil {
		return err
	}

	if _, err := b.Insert(ctx, orderLineRequest(w.warehouses, w.warehouseStart, loadDays)); err != nil {
		return err
	}

	if _, err := b.Insert(ctx, newOrderRequest(w.warehouses, w.warehouseStart)); err != nil {
		return err
	}

	return nil
}

func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	vs := w.vuState(b.VUID(), w.warehouseStart, w.warehouses)
	if w.variant == "procs" {
		return w.iterateProcs(ctx, b, vs)
	}

	return b.Step("workload", func() error {
		idx := weightedPick(vs.picker, txWeights)
		name := txNames[idx]

		if w.pacing {
			sleepSeconds(float64(keyingTime[name]))
		}

		switch idx {
		case 0:
			w.newOrder(ctx, b, vs)
		case 1:
			w.payment(ctx, b, vs)
		case 2:
			w.orderStatus(ctx, b, vs)
		case 3:
			w.delivery(ctx, b, vs)
		case 4:
			w.stockLevel(ctx, b, vs)
		}

		if w.pacing {
			sleepSeconds(thinkTime(vs.picker, thinkTimeMean[name]))
		}

		return nil
	})
}

func (*workload) Teardown(_ context.Context, _ *bench.Bench) error {
	// workload step tag is opened/closed per-iteration via Step("workload"); the
	// long-lived StepBegin in Setup is balanced here.
	return nil
}

// q fetches one named query, panicking on a missing section/query (a schema drift).
func (w *workload) q(section, name string) string {
	s, ok := w.sql.Query(section, name)
	if !ok {
		panic(fmt.Sprintf("tpcc: missing query %s/%s", section, name))
	}

	return s
}

func (w *workload) retryPolicy(b *bench.Bench) bench.RetryPolicy {
	return b.TxRetryPolicy(bench.TxRetryPolicyOptions{
		MaxAttempts: bench.EnvInt("RETRY_ATTEMPTS", 3),
		OnRetry:     func(int, error, bench.RetryDecision) { w.m.retryAttempts.Add(1) },
	})
}

// --- new_order ---

func (w *workload) newOrder(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.newOrderTotal.Add(1)

	start := time.Now()
	defer func() { w.m.newOrderDur.Add(float64(time.Since(start).Milliseconds())) }()

	wID := vs.homeWID
	dID := vs.ri(vs.noDID, 1, districtsPerWarehouse)
	cID := vs.nurand(vs.noCID, 1023, 1, customersPerDistrict, vs.noCIDSalt)
	olCnt := vs.ri(vs.noOlCnt, 5, 15)

	lineIID := make([]int64, olCnt)
	lineQty := make([]int64, olCnt)
	lineSupply := make([]int64, olCnt)
	allLocal := int64(1)

	for i := range olCnt {
		lineIID[i] = vs.nurand(vs.noItem, 8191, 1, items, vs.noItemSalt)
		lineQty[i] = vs.ri(vs.noQty, 1, 10)

		w.m.remoteLineTotal.Add(1)

		if w.warehouses > 1 && vs.ri(vs.noRemoteLine, 1, 100) <= 1 {
			w.m.remoteLineRemote.Add(1)

			lineSupply[i] = vs.pickRemoteWh()
			allLocal = 0
		} else {
			lineSupply[i] = wID
		}
	}

	forceRollback := vs.ri(vs.noRollback, 1, 100) <= 1 && w.iso != bench.IsoNone
	if forceRollback {
		w.m.rollbackDecided.Add(1)

		lineIID[olCnt-1] = items + 1 // nonexistent item → sentinel rollback
	}

	_ = bench.Retry0(ctx, w.retryPolicy(b), func() error {
		tx, err := b.Begin(ctx, bench.BeginOpts{Isolation: w.iso, Name: "new_order"})
		if err != nil {
			return err
		}

		if err := w.newOrderBody(ctx, tx, wID, dID, cID, olCnt, allLocal,
			lineIID, lineQty, lineSupply, forceRollback); err != nil {
			_ = tx.Rollback(ctx)

			if isRollbackSentinel(err) {
				return nil // spec-mandated rollback counts as success
			}

			return err
		}

		return tx.Commit(ctx)
	})
}

//nolint:gocognit,cyclop,funlen // TPC-C spec transaction; complexity is inherent to the spec.
func (w *workload) newOrderBody(
	ctx context.Context, tx *bench.TxX,
	wID, dID, cID, olCnt, allLocal int64,
	lineIID, lineQty, lineSupply []int64,
	forceRollback bool,
) error {
	if _, err := tx.QueryRow(ctx, w.q("workload_tx_new_order", "get_customer"), map[string]any{
		"c_id": cID, "d_id": dID, "w_id": wID,
	}); err != nil {
		return err
	}

	if _, err := tx.QueryRow(ctx, w.q("workload_tx_new_order", "get_warehouse"), map[string]any{"w_id": wID}); err != nil {
		return err
	}

	distRow, err := tx.QueryRow(ctx, w.q("workload_tx_new_order", "get_district"), map[string]any{
		"d_id": dID, "w_id": wID,
	})
	if err != nil {
		return err
	}

	if distRow == nil {
		return fmt.Errorf("%w: (%d,%d)", errDistrictNotFound, wID, dID)
	}

	oID := toInt64(distRow[0])

	if err := tx.Exec(ctx, w.q("workload_tx_new_order", "update_district"), map[string]any{
		"d_id": dID, "w_id": wID,
	}); err != nil {
		return err
	}

	if err := tx.Exec(ctx, w.q("workload_tx_new_order", "insert_order"), map[string]any{
		"o_id": oID, "d_id": dID, "w_id": wID, "c_id": cID,
		"ol_cnt": olCnt, "all_local": allLocal,
	}); err != nil {
		return err
	}

	if err := tx.Exec(ctx, w.q("workload_tx_new_order", "insert_new_order"), map[string]any{
		"o_id": oID, "d_id": dID, "w_id": wID,
	}); err != nil {
		return err
	}

	// Batch item read (IN list).
	uniqueIDs := uniqueInt64s(lineIID)

	itemRows, err := w.batchRead(ctx, tx, "workload_tx_new_order", "get_items_batch", "{item_ids}", wID, uniqueIDs, true)
	if err != nil {
		return err
	}

	itemMap := map[int64][]any{}
	for _, r := range itemRows {
		itemMap[toInt64(r[0])] = r
	}

	if forceRollback && !itemMapHas(itemMap, lineIID[olCnt-1]) {
		w.m.rollbackDone.Add(1)

		return errItemNotFound
	}

	// Batch stock read, grouped by supply warehouse.
	type stockKey struct{ wh, iid int64 }

	stockMap := map[stockKey][]any{}
	stockByWh := map[int64]map[int64]struct{}{}

	for i, iid := range lineIID {
		if !itemMapHas(itemMap, iid) {
			continue
		}

		sw := lineSupply[i]
		if stockByWh[sw] == nil {
			stockByWh[sw] = map[int64]struct{}{}
		}

		stockByWh[sw][iid] = struct{}{}
	}

	for sw, iids := range stockByWh {
		ids := make([]int64, 0, len(iids))
		for iid := range iids {
			ids = append(ids, iid)
		}

		rows, err := w.batchRead(ctx, tx, "workload_tx_new_order", "get_stocks_batch", "{item_ids}", sw, ids, false)
		if err != nil {
			return err
		}

		for _, r := range rows {
			stockMap[stockKey{sw, toInt64(r[0])}] = r
		}
	}

	for olNumber := int64(1); olNumber <= olCnt; olNumber++ {
		iid := lineIID[olNumber-1]
		olQuantity := lineQty[olNumber-1]
		supplyWID := lineSupply[olNumber-1]

		itemRow, ok := itemMap[iid]
		if !ok {
			w.m.rollbackDone.Add(1)

			return errItemNotFound
		}

		iPrice := toFloat64(itemRow[1])

		stockRow, ok := stockMap[stockKey{supplyWID, iid}]
		if !ok {
			continue // skip line if stock missing (matches tx.ts)
		}

		sQuantityOld := toInt64(stockRow[1])
		distCol := int(dID) + 2

		distInfo := ""
		if distCol < len(stockRow) {
			distInfo = toStr(stockRow[distCol])
		}

		newQuantity := sQuantityOld - olQuantity
		if newQuantity < 10 {
			newQuantity += 91
		}

		stockRow[1] = newQuantity // in-tx cache mutation for duplicate (wh,iid) pairs

		remoteCnt := int64(0)
		if supplyWID != wID {
			remoteCnt = 1
		}

		if err := tx.Exec(ctx, w.q("workload_tx_new_order", "update_stock"), map[string]any{
			"quantity": newQuantity, "ol_quantity": olQuantity,
			"remote_cnt": remoteCnt, "i_id": iid, "w_id": supplyWID,
		}); err != nil {
			return err
		}

		amount := math.Round(float64(olQuantity)*iPrice*100) / 100
		if err := tx.Exec(ctx, w.q("workload_tx_new_order", "insert_order_line"), map[string]any{
			"o_id": oID, "d_id": dID, "w_id": wID, "ol_number": olNumber,
			"i_id": iid, "supply_w_id": supplyWID, "quantity": olQuantity,
			"amount": amount, "dist_info": distInfo,
		}); err != nil {
			return err
		}
	}

	return nil
}

// batchRead issues a get_items_batch / get_stocks_batch query whose {item_ids}
// placeholder is either client-interpolated (pg/mysql/pico) or bound as a list (ydb).
func (w *workload) batchRead(
	ctx context.Context, tx *bench.TxX,
	section, query, placeholder string,
	wID int64, ids []int64, isItem bool,
) ([][]any, error) {
	tmpl := w.q(section, query)
	if w.isYdb {
		args := map[string]any{"item_ids": idsToIntAny(ids)}
		if !isItem {
			args["w_id"] = wID // stock query scopes by s_w_id; item query is global
		}

		return tx.QueryRows(ctx, tmpl, args)
	}

	rendered := strings.ReplaceAll(tmpl, placeholder, joinInt64s(ids))
	if isItem {
		return tx.QueryRows(ctx, rendered, nil) // get_items_batch has no :w_id
	}

	return tx.QueryRows(ctx, rendered, map[string]any{"w_id": wID})
}

// --- payment ---

//nolint:gocognit,cyclop // TPC-C spec transaction; complexity is inherent to the spec.
func (w *workload) payment(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.paymentTotal.Add(1)

	start := time.Now()
	defer func() { w.m.paymentDur.Add(float64(time.Since(start).Milliseconds())) }()

	wID := vs.homeWID
	dID := vs.ri(vs.payDID, 1, districtsPerWarehouse)
	amount := vs.rf(vs.payHAmount, 1, 5000)
	hData := vs.ascii(vs.payHData, 12, 24)
	hID := vs.nextHid()

	isRemote := w.warehouses > 1 && vs.ri(vs.payRemote, 1, 100) <= 15
	if isRemote {
		w.m.paymentRemote.Add(1)
	}

	cWID := wID
	if isRemote {
		cWID = vs.pickRemoteWh()
	}

	cDID := dID
	if isRemote {
		cDID = vs.ri(vs.payCDID, 1, districtsPerWarehouse)
	}

	isByName := vs.ri(vs.payByName, 1, 100) <= 60

	var cLastPick string
	if isByName {
		cLastPick = cLast(int(vs.nurand(vs.nurand255, 255, 0, 999, vs.nurand255Salt)))
	}

	cIDPick := vs.nurand(vs.payCID, 1023, 1, customersPerDistrict, vs.payCIDSalt) // always drained

	var wasBC bool

	_ = bench.Retry0(ctx, w.retryPolicy(b), func() error {
		wasBC = false

		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "payment"}, func(tx *bench.TxX) error {
			wName, err := w.paymentUpdateWarehouse(ctx, tx, wID, amount)
			if err != nil {
				return err
			}

			dName, err := w.paymentUpdateDistrict(ctx, tx, wID, dID, amount)
			if err != nil {
				return err
			}

			var (
				cID               int64
				cCredit, cDataOld string
			)

			if isByName { //nolint:nestif // TPC-C by-name customer lookup branch
				cnt, err := tx.QueryValue(ctx, w.q("workload_tx_payment", "count_customers_by_name"), map[string]any{
					"w_id": cWID, "d_id": cDID, "c_last": cLastPick,
				})
				if err != nil {
					return err
				}

				nameCount := toInt64(cnt)
				if nameCount == 0 {
					return fmt.Errorf("%w=%q in (%d,%d)", errPaymentNoCustomers, cLastPick, cWID, cDID)
				}

				offset := (nameCount - 1) / 2

				nameRow, err := w.customerByName(ctx, tx, "workload_tx_payment", cWID, cDID, cLastPick, offset)
				if err != nil {
					return err
				}

				if nameRow == nil {
					return fmt.Errorf("%w for c_last=%q", errPaymentByNameNoRow, cLastPick)
				}

				cID = toInt64(nameRow[0])
				cCredit = strings.TrimSpace(toStr(nameRow[10]))
				cDataOld = toStr(nameRow[15])
			} else {
				cID = cIDPick

				custRow, err := tx.QueryRow(ctx, w.q("workload_tx_payment", "get_customer_by_id"), map[string]any{
					"w_id": cWID, "d_id": cDID, "c_id": cID,
				})
				if err != nil {
					return err
				}

				if custRow == nil {
					return fmt.Errorf("%w: %d", errPaymentCustomerMissing, cID)
				}

				cCredit = strings.TrimSpace(toStr(custRow[9]))
				cDataOld = toStr(custRow[14])
			}

			if cCredit == "BC" {
				cDataNew := fmt.Sprintf("%d %d %d %d %d %s|%s", cID, cDID, cWID, dID, wID, fmtAmount(amount), cDataOld)
				if len(cDataNew) > 500 {
					cDataNew = cDataNew[:500]
				}

				if err := tx.Exec(ctx, w.q("workload_tx_payment", "update_customer_bc"), map[string]any{
					"w_id": cWID, "d_id": cDID, "c_id": cID,
					"amount": amount, "c_data_new": cDataNew,
				}); err != nil {
					return err
				}

				wasBC = true
			} else {
				if err := tx.Exec(ctx, w.q("workload_tx_payment", "update_customer"), map[string]any{
					"w_id": cWID, "d_id": cDID, "c_id": cID, "amount": amount,
				}); err != nil {
					return err
				}
			}

			hDataFull := wName + "    " + dName
			if len(hDataFull) > 24 {
				hDataFull = hDataFull[:24]
			}

			if hDataFull == "" {
				hDataFull = hData
			}

			return tx.Exec(ctx, w.q("workload_tx_payment", "insert_history"), map[string]any{
				"h_id": hID, "h_c_id": cID, "h_c_d_id": cDID, "h_c_w_id": cWID,
				"h_d_id": dID, "h_w_id": wID, "h_amount": amount, "h_data": hDataFull,
			})
		})
	})

	if isByName {
		w.m.paymentByname.Add(1)
	}

	if wasBC {
		w.m.paymentBc.Add(1)
	}
}

func (w *workload) paymentUpdateWarehouse(
	ctx context.Context, tx *bench.TxX, wID int64, amount float64,
) (string, error) {
	if w.hasReturning {
		row, err := tx.QueryRow(ctx, w.q("workload_tx_payment", "update_get_warehouse"), map[string]any{
			"w_id": wID, "amount": amount,
		})
		if err != nil {
			return "", err
		}

		if row == nil {
			return "", fmt.Errorf("%w: %d", errPaymentWarehouseMiss, wID)
		}

		return toStr(row[0]), nil
	}

	if err := tx.Exec(ctx, w.q("workload_tx_payment", "update_warehouse"), map[string]any{
		"w_id": wID, "amount": amount,
	}); err != nil {
		return "", err
	}

	row, err := tx.QueryRow(ctx, w.q("workload_tx_payment", "get_warehouse"), map[string]any{"w_id": wID})
	if err != nil {
		return "", err
	}

	if row == nil {
		return "", fmt.Errorf("%w: %d", errPaymentWarehouseMiss, wID)
	}

	return toStr(row[0]), nil
}

func (w *workload) paymentUpdateDistrict(
	ctx context.Context, tx *bench.TxX, wID, dID int64, amount float64,
) (string, error) {
	if w.hasReturning {
		row, err := tx.QueryRow(ctx, w.q("workload_tx_payment", "update_get_district"), map[string]any{
			"w_id": wID, "d_id": dID, "amount": amount,
		})
		if err != nil {
			return "", err
		}

		if row == nil {
			return "", fmt.Errorf("%w: (%d,%d)", errPaymentDistrictMiss, wID, dID)
		}

		return toStr(row[0]), nil
	}

	if err := tx.Exec(ctx, w.q("workload_tx_payment", "update_district"), map[string]any{
		"w_id": wID, "d_id": dID, "amount": amount,
	}); err != nil {
		return "", err
	}

	row, err := tx.QueryRow(ctx, w.q("workload_tx_payment", "get_district"), map[string]any{"w_id": wID, "d_id": dID})
	if err != nil {
		return "", err
	}

	if row == nil {
		return "", fmt.Errorf("%w: (%d,%d)", errPaymentDistrictMiss, wID, dID)
	}

	return toStr(row[0]), nil
}

// customerByName resolves the median customer by c_last. Picodata has no OFFSET, so
// it fetches all rows and picks the median client-side; other drivers use :offset.
func (w *workload) customerByName(
	ctx context.Context, tx *bench.TxX, section string,
	wID, dID int64, cLast string, offset int64,
) ([]any, error) {
	if w.isPicodata {
		rows, err := tx.QueryRows(ctx, w.q(section, "get_customer_by_name"), map[string]any{
			"w_id": wID, "d_id": dID, "c_last": cLast,
		})
		if err != nil {
			return nil, err
		}

		if int(offset) >= len(rows) {
			return nil, nil
		}

		return rows[offset], nil
	}

	return tx.QueryRow(ctx, w.q(section, "get_customer_by_name"), map[string]any{
		"w_id": wID, "d_id": dID, "c_last": cLast, "offset": offset,
	})
}

// --- order_status (read-only) ---

//nolint:gocognit,cyclop // TPC-C spec transaction; complexity is inherent to the spec.
func (w *workload) orderStatus(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.orderStatusTotal.Add(1)

	start := time.Now()
	defer func() { w.m.orderStatusDur.Add(float64(time.Since(start).Milliseconds())) }()

	wID := vs.homeWID
	dID := vs.ri(vs.osDID, 1, districtsPerWarehouse)
	cIDPick := vs.nurand(vs.osCID, 1023, 1, customersPerDistrict, vs.osCIDSalt)
	isByName := vs.ri(vs.osByName, 1, 100) <= 60

	var cLastPick string
	if isByName {
		cLastPick = cLast(int(vs.nurand(vs.nurand255, 255, 0, 999, vs.nurand255Salt)))
	}

	bynameObserved := false
	_ = bench.Retry0(ctx, w.retryPolicy(b), func() error {
		bynameObserved = false

		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "order_status"}, func(tx *bench.TxX) error {
			var cID int64

			if isByName { //nolint:nestif // TPC-C by-name customer lookup branch
				cnt, err := tx.QueryValue(ctx, w.q("workload_tx_order_status", "count_customers_by_name"), map[string]any{
					"w_id": wID, "d_id": dID, "c_last": cLastPick,
				})
				if err != nil {
					return err
				}

				if toInt64(cnt) == 0 {
					return nil
				}

				offset := (toInt64(cnt) - 1) / 2

				nameRow, err := w.customerByName(ctx, tx, "workload_tx_order_status", wID, dID, cLastPick, offset)
				if err != nil {
					return err
				}

				if nameRow == nil {
					return nil
				}

				cID = toInt64(nameRow[len(nameRow)-1])
				bynameObserved = true
			} else {
				cID = cIDPick

				custRow, err := tx.QueryRow(ctx, w.q("workload_tx_order_status", "get_customer_by_id"), map[string]any{
					"c_id": cID, "d_id": dID, "w_id": wID,
				})
				if err != nil {
					return err
				}

				if custRow == nil {
					return nil
				}
			}

			lastRow, err := tx.QueryRow(ctx, w.q("workload_tx_order_status", "get_last_order"), map[string]any{
				"d_id": dID, "w_id": wID, "c_id": cID,
			})
			if err != nil {
				return err
			}

			if lastRow == nil {
				return nil
			}

			oID := toInt64(lastRow[0])
			_, _ = tx.QueryRows(ctx, w.q("workload_tx_order_status", "get_order_lines"), map[string]any{
				"o_id": oID, "d_id": dID, "w_id": wID,
			})

			return nil
		})
	})

	if bynameObserved {
		w.m.orderStatusByname.Add(1)
	}
}

// --- delivery ---

//nolint:gocognit,cyclop // TPC-C spec transaction; complexity is inherent to the spec.
func (w *workload) delivery(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.deliveryTotal.Add(1)

	start := time.Now()
	defer func() { w.m.deliveryDur.Add(float64(time.Since(start).Milliseconds())) }()

	wID := vs.homeWID
	carrierID := vs.ri(vs.dCarrier, 1, 10)

	_ = bench.Retry0(ctx, w.retryPolicy(b), func() error {
		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "delivery"}, func(tx *bench.TxX) error {
			for dID := int64(1); dID <= districtsPerWarehouse; dID++ {
				minRow, err := tx.QueryRow(ctx, w.q("workload_tx_delivery", "get_min_new_order"), map[string]any{
					"d_id": dID, "w_id": wID,
				})
				if err != nil {
					return err
				}

				if len(minRow) == 0 || minRow[0] == nil {
					continue
				}

				oID := toInt64(minRow[0])
				if err := tx.Exec(ctx, w.q("workload_tx_delivery", "delete_new_order"), map[string]any{
					"o_id": oID, "d_id": dID, "w_id": wID,
				}); err != nil {
					return err
				}

				orderRow, err := tx.QueryRow(ctx, w.q("workload_tx_delivery", "get_order"), map[string]any{
					"o_id": oID, "d_id": dID, "w_id": wID,
				})
				if err != nil {
					return err
				}

				if orderRow == nil {
					continue
				}

				cID := toInt64(orderRow[0])

				if err := tx.Exec(ctx, w.q("workload_tx_delivery", "update_order"), map[string]any{
					"carrier_id": carrierID, "o_id": oID, "d_id": dID, "w_id": wID,
				}); err != nil {
					return err
				}

				if err := tx.Exec(ctx, w.q("workload_tx_delivery", "update_order_line"), map[string]any{
					"o_id": oID, "d_id": dID, "w_id": wID,
				}); err != nil {
					return err
				}

				sumRow, err := tx.QueryRow(ctx, w.q("workload_tx_delivery", "get_order_line_amount"), map[string]any{
					"o_id": oID, "d_id": dID, "w_id": wID,
				})
				if err != nil {
					return err
				}

				olTotal := int64(0)
				if len(sumRow) > 0 && sumRow[0] != nil {
					olTotal = toInt64(sumRow[0])
				}

				if err := tx.Exec(ctx, w.q("workload_tx_delivery", "update_customer"), map[string]any{
					"amount": olTotal, "c_id": cID, "d_id": dID, "w_id": wID,
				}); err != nil {
					return err
				}
			}

			return nil
		})
	})
}

// --- stock_level (read-only) ---

//nolint:gocognit,cyclop // TPC-C spec transaction; complexity is inherent to the spec.
func (w *workload) stockLevel(ctx context.Context, b *bench.Bench, vs *vuState) {
	w.m.stockLevelTotal.Add(1)

	start := time.Now()
	defer func() { w.m.stockLevelDur.Add(float64(time.Since(start).Milliseconds())) }()

	wID := vs.homeWID
	dID := vs.ri(vs.slDID, 1, districtsPerWarehouse)
	threshold := vs.ri(vs.slThreshold, 10, 20)

	_ = bench.Retry0(ctx, w.retryPolicy(b), func() error {
		return b.BeginTx(ctx, bench.BeginOpts{Isolation: w.iso, Name: "stock_level"}, func(tx *bench.TxX) error {
			nextOIDv, err := tx.QueryValue(ctx, w.q("workload_tx_stock_level", "get_district"), map[string]any{
				"w_id": wID, "d_id": dID,
			})
			if err != nil {
				return err
			}

			if nextOIDv == nil {
				return nil
			}

			nextOID := toInt64(nextOIDv)

			olRows, err := tx.QueryRows(ctx, w.q("workload_tx_stock_level", "get_window_items"), map[string]any{
				"w_id": wID, "d_id": dID,
				"min_o_id": nextOID - 20, "next_o_id": nextOID,
			})
			if err != nil {
				return err
			}

			if len(olRows) == 0 {
				return nil
			}

			ids := make([]int64, 0, len(olRows))
			for _, r := range olRows {
				if len(r) > 0 && r[0] != nil {
					ids = append(ids, toInt64(r[0]))
				}
			}

			if len(ids) == 0 {
				return nil
			}

			tmpl := w.q("workload_tx_stock_level", "stock_count_in")
			if w.isYdb {
				_, _ = tx.QueryValue(ctx, tmpl, map[string]any{"w_id": wID, "threshold": threshold, "ids": idsToIntAny(ids)})
			} else {
				rendered := strings.ReplaceAll(tmpl, "{ids}", joinInt64s(ids))
				_, _ = tx.QueryValue(ctx, rendered, map[string]any{"w_id": wID, "threshold": threshold})
			}

			return nil
		})
	})
}

// --- per-VU state + tx-time generators ---

type vuState struct {
	homeWID        int64
	warehouses     int64
	warehouseStart int64

	picker *rand.Rand

	noDID, noOlCnt, noQty, noRemoteLine, noRollback *rand.Rand
	noCID, noItem                                   *rand.Rand
	noCIDSalt, noItemSalt                           uint64

	payDID, payCDID, payHAmount, payHData, payRemote, payByName *rand.Rand
	payCID                                                      *rand.Rand
	payCIDSalt                                                  uint64

	osDID, osByName *rand.Rand
	osCID           *rand.Rand
	osCIDSalt       uint64

	dCarrier *rand.Rand

	slDID, slThreshold *rand.Rand

	nurand255     *rand.Rand
	nurand255Salt uint64
	remoteWh      *rand.Rand

	hid atomic.Int64
}

func (w *workload) vuState(vuid uint64, warehouseStart, warehouses int64) *vuState {
	if v, ok := w.vuStates.Load(vuid); ok {
		vs, _ := v.(*vuState) //nolint:errcheck // vuStates only stores *vuState values

		return vs
	}

	newRand := func(slot string) *rand.Rand {
		s := seedOf(slot, vuid)

		return rand.New(rand.NewPCG(s, s)) //nolint:gosec // G404: workload data RNG, cryptographic strength not required
	}
	vs := &vuState{
		homeWID:        warehouseStart + (int64(vuid)-1)%warehouses, //nolint:gosec // G115: scale-bound, no overflow
		warehouses:     warehouses,
		warehouseStart: warehouseStart,
		picker:         newRand("picker"),
		noDID:          newRand("neword.d_id"),
		noCID:          newRand("neword.c_id"),
		noOlCnt:        newRand("neword.ol_cnt"),
		noItem:         newRand("neword.item_id"),
		noQty:          newRand("neword.quantity"),
		noRemoteLine:   newRand("neword.remote_line"),
		noRollback:     newRand("neword.rollback"),
		payDID:         newRand("payment.d_id"),
		payCDID:        newRand("payment.c_d_id"),
		payCID:         newRand("payment.c_id"),
		payHAmount:     newRand("payment.h_amount"),
		payHData:       newRand("payment.h_data"),
		payRemote:      newRand("payment.remote"),
		payByName:      newRand("payment.byname"),
		osDID:          newRand("ostat.d_id"),
		osCID:          newRand("ostat.c_id"),
		osByName:       newRand("ostat.byname"),
		dCarrier:       newRand("delivery.o_carrier_id"),
		slDID:          newRand("slev.d_id"),
		slThreshold:    newRand("slev.threshold"),
		nurand255:      newRand("nurand255"),
		remoteWh:       newRand("remoteWh"),
		noCIDSalt:      seedOf("neword.c_id", vuid),
		noItemSalt:     seedOf("neword.item_id", vuid),
		payCIDSalt:     seedOf("payment.c_id", vuid),
		osCIDSalt:      seedOf("ostat.c_id", vuid),
		nurand255Salt:  seedOf("nurand255", vuid),
	}
	vs.hid.Store(int64(vuid) * 10_000_000) //nolint:gosec // G115: value bounded by scale factor, no overflow path
	actual, _ := w.vuStates.LoadOrStore(vuid, vs)
	stored, _ := actual.(*vuState) //nolint:errcheck // vuStates only stores *vuState values

	return stored
}

func (v *vuState) nextHid() int64 { return v.hid.Add(1) }

// pickRemoteWh returns a uniform warehouse other than homeWID (only when W>1).
// It draws from [1, W-1], shifts into the global warehouse range, and skips homeWID.
func (v *vuState) pickRemoteWh() int64 {
	alt := v.ri(v.remoteWh, 1, int(v.warehouses-1)) + v.warehouseStart - 1
	if alt >= v.homeWID {
		alt++
	}

	return alt
}

// ri draws a uniform int in [lo,hi].
func (v *vuState) ri(r *rand.Rand, lo, hi int) int64 { return int64(r.IntN(hi-lo+1) + lo) }

// rf draws a uniform float in [lo,hi).
func (v *vuState) rf(r *rand.Rand, lo, hi float64) float64 { return lo + r.Float64()*(hi-lo) }

// ascii draws a random [min,max]-length ASCII string over [a-zA-Z].
func (v *vuState) ascii(r *rand.Rand, minLen, maxLen int) string {
	n := v.ri(r, minLen, maxLen)

	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[r.IntN(len(alphabet))]
	}

	return string(b)
}

// nurand is the §2.1.6 non-uniform draw (helpers.go), wrapped here as a method.
func (v *vuState) nurand(r *rand.Rand, paramA, lo, hi int, cSalt uint64) int64 {
	return int64(nurand(r, paramA, lo, hi, cSalt))
}

// --- helpers ---

func isRollbackSentinel(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "tpcc_rollback:")
}

func itemMapHas(m map[int64][]any, iid int64) bool {
	_, ok := m[iid]

	return ok
}

func uniqueInt64s(in []int64) []int64 {
	seen := map[int64]struct{}{}

	out := make([]int64, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}

	return out
}

func joinInt64s(in []int64) string {
	var b strings.Builder

	for i, v := range in {
		if i > 0 {
			b.WriteByte(',')
		}

		fmt.Fprintf(&b, "%d", v)
	}

	return b.String()
}

func idsToIntAny(in []int64) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}

	return out
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func fmtAmount(amount float64) string {
	// tx.ts used amount.toFixed(2) on the float draw; mirror the 2-decimal form.
	return fmt.Sprintf("%.2f", amount)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}

	return "false"
}

func sleepSeconds(s float64) {
	if s <= 0 {
		return
	}

	time.Sleep(time.Duration(s * float64(time.Second)))
}

// thinkTime draws a negative-exponential delay truncated at 10× the mean.
func thinkTime(r *rand.Rand, mean float64) float64 {
	if mean <= 0 {
		return 0
	}

	t := -math.Log(1-r.Float64()) * mean
	if capVal := 10 * mean; t > capVal {
		t = capVal
	}

	return t
}

// seedOf mirrors tpcc_common.seedOf: a per-VU, per-slot offset so concurrent VUs
// draw independent sequences.
func seedOf(slot string, vuid uint64) uint64 {
	var h uint32
	for _, c := range slot {
		h = h*131 + uint32(c) //nolint:gosec // G115: value bounded by scale factor, no overflow path
	}

	return (vuid * 0x9e3779b9) ^ uint64(h)
}
