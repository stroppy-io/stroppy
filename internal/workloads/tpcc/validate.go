package tpcc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

var errValidatePopulation = errors.New("validate_population")

// validatePopulation runs the §1.3.1 consistency/cardinality checks and aborts
// (returns an error) if any fail, mirroring validatePopulation in tpcc_common.ts.
func validatePopulation(ctx context.Context, b *bench.Bench, warehouses, warehouseStart, wIDMax int64) error {
	wRange := fmt.Sprintf("BETWEEN %d AND %d", warehouseStart, wIDMax)
	wWhere := func(col string) string { return "WHERE " + col + " " + wRange }

	var failures []string

	check := func(name string, ok bool) {
		if !ok {
			failures = append(failures, name)
		}
	}

	distNext, ordMax, noStats, err := prefetchDistrictAggregates(ctx, b, wWhere)
	if err != nil {
		return err
	}

	cc1WSum, _ := qfloat(ctx, b, "SELECT SUM(w_ytd) FROM warehouse WHERE w_id "+wRange)
	cc1DSum, _ := qfloat(ctx, b, "SELECT SUM(d_ytd) FROM district "+wWhere("d_w_id"))
	cc4OSum, _ := qint(ctx, b, "SELECT SUM(o_ol_cnt) FROM orders "+wWhere("o_w_id"))
	cc4OlCnt, _ := qint(ctx, b, "SELECT COUNT(*) FROM order_line "+wWhere("ol_w_id"))

	checkCardinalities(ctx, b, check, wWhere, wRange, warehouses)
	checkConsistency(check, distNext, ordMax, noStats, cc1WSum, cc1DSum, cc4OSum, cc4OlCnt)
	checkDistribution(ctx, b, check, wWhere, wRange)

	if len(failures) > 0 {
		detail := strings.Join(failures, "\n  ")

		return fmt.Errorf("%w: %d check(s) failed:\n  %s", errValidatePopulation, len(failures), detail)
	}

	return nil
}

// prefetchDistrictAggregates loads the per-district next-o-id, max order id, and
// new_order min/max/count aggregates used by the consistency checks (CC2/CC3).
func prefetchDistrictAggregates(
	ctx context.Context, b *bench.Bench,
	wWhere func(string) string,
) (distNext, ordMax map[string]int64, noStats map[string]noStat, err error) {
	distRows, err := b.QueryRows(ctx, "SELECT d_w_id, d_id, d_next_o_id FROM district "+wWhere("d_w_id"), nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("validate_population: prefetch failed: %w", err)
	}

	distNext = map[string]int64{}
	for _, r := range distRows {
		distNext[fmt.Sprintf("%v/%v", r[0], r[1])] = toInt64(r[2])
	}

	ordRows, err := b.QueryRows(ctx,
		"SELECT o_w_id, o_d_id, MAX(o_id) FROM orders "+wWhere("o_w_id")+
			" GROUP BY o_w_id, o_d_id", nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("validate_population: prefetch failed: %w", err)
	}

	ordMax = map[string]int64{}
	for _, r := range ordRows {
		ordMax[fmt.Sprintf("%v/%v", r[0], r[1])] = toInt64(r[2])
	}

	noRows, err := b.QueryRows(ctx, "SELECT no_w_id, no_d_id, MAX(no_o_id), MIN(no_o_id), COUNT(*) FROM new_order "+
		wWhere("no_w_id")+" GROUP BY no_w_id, no_d_id", nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("validate_population: prefetch failed: %w", err)
	}

	noStats = map[string]noStat{}
	for _, r := range noRows {
		noStats[fmt.Sprintf("%v/%v", r[0], r[1])] = noStat{toInt64(r[2]), toInt64(r[3]), toInt64(r[4])}
	}

	return distNext, ordMax, noStats, nil
}

type noStat struct{ max, min, cnt int64 }

// checkCardinalities runs the eight §1.3.1 table-cardinality checks.
func checkCardinalities(
	ctx context.Context, b *bench.Bench,
	check func(string, bool),
	wWhere func(string) string, wRange string, warehouses int64,
) {
	totalDistricts := warehouses * districtsPerWarehouse
	totalCustomers := warehouses * customersPerWh
	totalStock := warehouses * itemsPerWh
	totalOrders := totalCustomers
	totalNewOrder := totalDistricts * ordersUndelivered
	totalOrderLine := totalOrders * olCntFixed

	check("ITEM = 100000", qintEq(ctx, b, "SELECT COUNT(*) FROM item", items))
	check("WAREHOUSE = WAREHOUSES", qintEq(ctx, b, "SELECT COUNT(*) FROM warehouse WHERE w_id "+wRange, warehouses))
	check("DISTRICT = TOTAL_DISTRICTS", qintEq(ctx, b, "SELECT COUNT(*) FROM district "+wWhere("d_w_id"), totalDistricts))
	check("CUSTOMER = TOTAL_CUSTOMERS", qintEq(ctx, b, "SELECT COUNT(*) FROM customer "+wWhere("c_w_id"), totalCustomers))
	check("STOCK = TOTAL_STOCK", qintEq(ctx, b, "SELECT COUNT(*) FROM stock "+wWhere("s_w_id"), totalStock))
	check("ORDERS = TOTAL_ORDERS", qintEq(ctx, b, "SELECT COUNT(*) FROM orders "+wWhere("o_w_id"), totalOrders))
	check("NEW_ORDER = TOTAL_NEW_ORDER", qintEq(ctx, b,
		"SELECT COUNT(*) FROM new_order "+wWhere("no_w_id"), totalNewOrder))
	check("ORDER_LINE = TOTAL_ORDER_LINE", qintEq(ctx, b,
		"SELECT COUNT(*) FROM order_line "+wWhere("ol_w_id"), totalOrderLine))
}

// checkConsistency runs the CC1–CC4 logical-consistency checks against the
// prefetched district/order/new_order aggregates.
func checkConsistency(
	check func(string, bool),
	distNext map[string]int64, ordMax map[string]int64, noStats map[string]noStat,
	cc1WSum, cc1DSum float64, cc4OSum, cc4OlCnt int64,
) {
	check("CC1 sum(W_YTD) = sum(D_YTD)", absf(cc1WSum-cc1DSum) < 0.01)

	for k, dNext := range distNext {
		check("CC2a D_NEXT_O_ID-1 = max(O_ID) ["+k+"]", ordMax[k] == dNext-1)
		check("CC2b max(O_ID) = max(NO_O_ID) ["+k+"]", ordMax[k] == noStats[k].max)
		st := noStats[k]
		check("CC3 new_order contiguous ["+k+"]", st.max-st.min+1 == st.cnt)
	}

	check("CC4 sum(O_OL_CNT) = count(order_line)", cc4OSum == cc4OlCnt)
}

// checkDistribution runs the §1.3.1 data-distribution and constant-column checks.
func checkDistribution(
	ctx context.Context, b *bench.Bench,
	check func(string, bool),
	wWhere func(string) string, wRange string,
) {
	iDataPct, _ := qfloat(ctx, b,
		"SELECT 100.0 * SUM(CASE WHEN i_data LIKE '%ORIGINAL%' THEN 1 ELSE 0 END) / COUNT(*) FROM item")
	check("I_DATA 10% ORIGINAL (5..15%)", iDataPct >= 5 && iDataPct <= 15)

	sDataPct, _ := qfloat(ctx, b,
		"SELECT 100.0 * SUM(CASE WHEN s_data LIKE '%ORIGINAL%' THEN 1 ELSE 0 END) / COUNT(*) FROM stock "+
			wWhere("s_w_id"))
	check("S_DATA 10% ORIGINAL (5..15%)", sDataPct >= 5 && sDataPct <= 15)

	bcPct, _ := qfloat(ctx, b,
		"SELECT 100.0 * SUM(CASE WHEN c_credit = 'BC' THEN 1 ELSE 0 END) / COUNT(*) FROM customer "+
			wWhere("c_w_id"))
	check("C_CREDIT 10% BC (5..15%)", bcPct >= 5 && bcPct <= 15)
	check("C_MIDDLE = 'OE' everywhere", qintEq(ctx, b,
		"SELECT COUNT(*) FROM customer WHERE c_middle <> 'OE' AND c_w_id "+wRange, 0))
	check("W_YTD = 300000 everywhere", qintEq(ctx, b,
		"SELECT COUNT(*) FROM warehouse WHERE w_ytd <> 300000 AND w_id "+wRange, 0))
	check("D_NEXT_O_ID = 3001 everywhere", qintEq(ctx, b,
		"SELECT COUNT(*) FROM district WHERE d_next_o_id <> 3001 AND d_w_id "+wRange, 0))
}

// qint runs a scalar COUNT/SUM-int query and compares to want.
func qintEq(ctx context.Context, b *bench.Bench, sql string, want int64) bool {
	got, err := qint(ctx, b, sql)

	return err == nil && got == want
}

func qint(ctx context.Context, b *bench.Bench, sql string) (int64, error) {
	v, err := b.QueryValue(ctx, sql, nil)
	if err != nil || v == nil {
		return 0, err
	}

	return toInt64(v), nil
}

func qfloat(ctx context.Context, b *bench.Bench, sql string) (float64, error) {
	v, err := b.QueryValue(ctx, sql, nil)
	if err != nil || v == nil {
		return 0, err
	}

	return toFloat64(v), nil
}

// toInt64 coerces driver-returned scalars. pgx returns NUMERIC/DECIMAL aggregates
// as decimal types not covered by the plain switch, so the default stringifies and
// re-parses (the decimal's String form is plain digits).
func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int32:
		return int64(x)
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	case []byte:
		return atoi(string(x))
	case string:
		return atoi(x)
	default:
		return atoi(fmt.Sprint(x))
	}
}

func toFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int32:
		return float64(x)
	case int:
		return float64(x)
	case []byte:
		return atof(string(x))
	case string:
		return atof(x)
	case time.Time:
		return float64(x.Unix())
	default:
		return atof(fmt.Sprint(x))
	}
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}

	return f
}

func atoi(s string) int64 {
	var n int64

	neg := false

	for i := range len(s) {
		c := s[i]
		if c == '-' {
			neg = true

			continue
		}

		if c == '.' {
			break
		}

		if c < '0' || c > '9' {
			continue
		}

		n = n*10 + int64(c-'0')
	}

	if neg {
		return -n
	}

	return n
}

func atof(s string) float64 {
	var (
		f    float64
		frac float64
	)

	div := 1.0
	neg := false
	seenDot := false

	for i := range len(s) {
		c := s[i]
		if c == '-' {
			neg = true

			continue
		}

		if c == '.' {
			seenDot = true

			continue
		}

		if c < '0' || c > '9' {
			continue
		}

		if seenDot {
			div *= 10
			frac = frac*10 + float64(c-'0')
		} else {
			f = f*10 + float64(c-'0')
		}
	}

	f += frac / div
	if neg {
		return -f
	}

	return f
}
