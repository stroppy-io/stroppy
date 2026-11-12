package tpch

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch/dbgen"
	"github.com/stroppy-io/stroppy/v6/workloads/tpch/tpchgen"
)

var (
	refreshOrderColumns = []string{
		"o_orderkey", "o_custkey", "o_orderstatus", "o_totalprice", "o_orderdate",
		"o_orderpriority", "o_clerk", "o_shippriority", "o_comment",
	}
	refreshLineColumns = []string{
		"l_orderkey", "l_partkey", "l_suppkey", "l_linenumber", "l_quantity", "l_extendedprice",
		"l_discount", "l_tax", "l_returnflag", "l_linestatus", "l_shipdate", "l_commitdate",
		"l_receiptdate", "l_shipinstruct", "l_shipmode", "l_comment",
	}
)

func refreshRows(order *dbgen.Order) (row []any, lines [][]any) {
	row = []any{
		int64(order.OKey), int64(order.CustKey), order.Status, dbgen.Money(int64(order.TotalPrice)),
		order.Date, order.OrderPriority, order.Clerk, order.ShipPriority, order.Comment,
	}

	lines = make([][]any, 0, len(order.Lines))
	for i := range order.Lines {
		line := &order.Lines[i]
		lines = append(lines, []any{
			int64(line.OKey), int64(line.PartKey), int64(line.SuppKey), int64(line.LCnt), int64(line.Quantity),
			dbgen.Money(int64(line.EPrice)), dbgen.Money(int64(line.Discount)), dbgen.Money(int64(line.Tax)),
			line.RFlag, line.LStatus, line.SDate, line.CDate, line.RDate, line.ShipInstruct, line.ShipMode, line.Comment,
		})
	}

	return row, lines
}

func insertRefresh(
	ctx context.Context, session *bench.Session, generator *tpchgen.RefreshGenerator, attempt *refreshAttempt,
) error {
	for {
		orders, lines := make([][]any, 0, refreshBatchOrders), make([][]any, 0, refreshBatchOrders*4)
		for range refreshBatchOrders {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			order, ok := generator.Next()
			if !ok {
				break
			}

			row, detail := refreshRows(order)
			orders, lines = append(orders, row), append(lines, detail...)
		}

		if len(orders) == 0 {
			return nil
		}

		err := session.BeginTx(ctx, bench.BeginOpts{Isolation: bench.IsoSerializable, Name: "tpch_rf1"},
			func(tx *bench.Tx) error {
				if err := insertRefreshRows(ctx, tx, "orders", refreshOrderColumns, orders); err != nil {
					return err
				}

				return insertRefreshRows(ctx, tx, "lineitem", refreshLineColumns, lines)
			})
		if err != nil {
			return fmt.Errorf("RF1 set %d: %w", attempt.Set, err)
		}

		attempt.Orders += int64(len(orders))
		attempt.Lineitems += int64(len(lines))
	}
}

func insertRefreshRows(ctx context.Context, tx *bench.Tx, table string, columns []string, rows [][]any) error {
	groups := make([]string, len(rows))

	args := make(map[string]any, len(rows)*len(columns))
	for i, row := range rows {
		parameters := make([]string, len(row))
		for j, value := range row {
			name := fmt.Sprintf("p%d_%d", i, j)
			parameters[j], args[name] = ":"+name, value
		}

		groups[i] = "(" + strings.Join(parameters, ", ") + ")"
	}

	return tx.Exec(ctx, "INSERT INTO "+table+" ("+strings.Join(columns, ",")+") VALUES "+strings.Join(groups, ","), args)
}

func deleteRefresh(
	ctx context.Context, session *bench.Session, generator *tpchgen.RefreshGenerator, attempt *refreshAttempt,
) error {
	for start := int64(0); start < generator.Count(); start += refreshBatchOrders {
		count := min(int64(refreshBatchOrders), generator.Count()-start)

		keys, args := make([]string, count), make(map[string]any, count)
		for i := range count {
			name := fmt.Sprintf("key%d", i)
			keys[i], args[name] = ":"+name, generator.DeleteKey(start+i)
		}

		predicate := " IN (" + strings.Join(keys, ", ") + ")"

		var lines int64

		err := session.BeginTx(ctx, bench.BeginOpts{Isolation: bench.IsoSerializable, Name: "tpch_rf2"},
			func(tx *bench.Tx) error {
				var err error

				lines, err = deleteRefreshBatch(ctx, tx, predicate, args, count)

				return err
			})
		if err != nil {
			return fmt.Errorf("RF2 set %d: %w", attempt.Set, err)
		}

		attempt.Orders += count
		attempt.Lineitems += lines
	}

	return nil
}

func refreshCount(ctx context.Context, tx *bench.Tx, query string, args map[string]any) (int64, error) {
	row, err := tx.RawRow(ctx, query, args)
	if err != nil {
		return 0, err
	}

	if len(row) != 1 {
		return 0, errRefreshData
	}

	return strconv.ParseInt(fmt.Sprint(row[0]), 10, 64)
}

func deleteRefreshBatch(
	ctx context.Context, tx *bench.Tx, predicate string, args map[string]any, count int64,
) (int64, error) {
	orders, err := refreshCount(ctx, tx, "SELECT COUNT(*) FROM orders WHERE o_orderkey"+predicate, args)
	if err != nil {
		return 0, err
	}

	if orders != count {
		return 0, fmt.Errorf("%w: expected %d old orders, got %d", errRefreshData, count, orders)
	}

	matched, err := refreshCount(ctx, tx,
		"SELECT COUNT(DISTINCT l_orderkey) FROM lineitem WHERE l_orderkey"+predicate, args)
	if err != nil {
		return 0, err
	}

	if matched != count {
		return 0, fmt.Errorf("%w: missing old lineitems", errRefreshData)
	}

	lines, err := refreshCount(ctx, tx, "SELECT COUNT(*) FROM lineitem WHERE l_orderkey"+predicate, args)
	if err != nil {
		return 0, err
	}

	if err := tx.Exec(ctx, "DELETE FROM lineitem WHERE l_orderkey"+predicate, args); err != nil {
		return 0, err
	}

	return lines, tx.Exec(ctx, "DELETE FROM orders WHERE o_orderkey"+predicate, args)
}
