package main

import (
	"fmt"
	"log"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// rawQuery parses a single parameterless SQL statement into a *sqlfile.Query for
// ad-hoc validation/reporting queries (not part of the embedded corpus).
func rawQuery(sqlText string) *sqlfile.Query {
	f, err := sqlfile.Parse([]byte("--+ s\n--= q\n" + sqlText))
	if err != nil {
		log.Fatalf("tpcc: bad internal query %q: %v", sqlText, err)
	}
	q, _ := f.Query("s", "q")
	return q
}

// scalarInt runs a scalar-returning query on conn and returns the first column.
func scalarInt(vu *bench.VU, conn driver.Conn, sqlText string) (int64, error) {
	st, err := conn.Prepare(vu.Ctx(), rawQuery(sqlText))
	if err != nil {
		return 0, err
	}
	return conn.QueryRow(vu.Ctx(), st).ScanInt64(0)
}

// validatePopulation checks the loaded row counts against the §4.3.3 formulas and
// the cheap post-load consistency conditions (§3.3.2). It is a run-once step,
// gated by VALIDATE.
func validatePopulation(w *world) bench.Handler {
	return bench.FuncOnce(func(vu *bench.VU) error {
		conn := vu.Conn()
		wn := w.warehouses

		counts := []struct {
			name string
			sql  string
			want int64
		}{
			{"item", "SELECT count(*) FROM item", itemsCount},
			{"warehouse", "SELECT count(*) FROM warehouse", wn},
			{"district", "SELECT count(*) FROM district", wn * districtsPerWarehouse},
			{"customer", "SELECT count(*) FROM customer", wn * districtsPerWarehouse * customersPerDistrict},
			{"history", "SELECT count(*) FROM history", wn * districtsPerWarehouse * customersPerDistrict},
			{"stock", "SELECT count(*) FROM stock", wn * stockPerWarehouse},
			{"orders", "SELECT count(*) FROM orders", wn * districtsPerWarehouse * ordersPerDistrict},
			{"new_order", "SELECT count(*) FROM new_order", wn * districtsPerWarehouse * newOrdersPerDistrict},
		}
		for _, c := range counts {
			got, err := scalarInt(vu, conn, c.sql)
			if err != nil {
				return fmt.Errorf("count %s: %w", c.name, err)
			}
			if got != c.want {
				return fmt.Errorf("%s count = %d, want %d", c.name, got, c.want)
			}
		}

		// Consistency condition 4: sum(o_ol_cnt) == count(order_line).
		ol, err := scalarInt(vu, conn, "SELECT count(*) FROM order_line")
		if err != nil {
			return err
		}
		sum, err := scalarInt(vu, conn, "SELECT coalesce(sum(o_ol_cnt),0) FROM orders")
		if err != nil {
			return err
		}
		if ol != sum {
			return fmt.Errorf("order_line count %d != sum(o_ol_cnt) %d", ol, sum)
		}

		// Consistency condition 1: per warehouse, w_ytd == sum(d_ytd).
		if err := checkWYTD(vu, conn); err != nil {
			return err
		}

		// Consistency condition (delivered split): every undelivered order
		// (o_id >= 2101) has a matching new_order row and vice-versa per district.
		bad, err := scalarInt(vu, conn,
			"SELECT count(*) FROM orders WHERE (o_carrier_id IS NULL) <> (o_id >= 2101)")
		if err != nil {
			return err
		}
		if bad != 0 {
			return fmt.Errorf("delivered/undelivered carrier split violated in %d orders", bad)
		}

		log.Printf("[tpcc] validate_population: OK (W=%d)", wn)
		return nil
	})
}

// checkConsistency runs the post-workload consistency subset: condition 1
// (w_ytd == sum(d_ytd)) is invariant under the workload because Payment credits a
// warehouse and one of its districts by the same amount.
func checkConsistency() bench.Handler {
	return bench.FuncOnce(func(vu *bench.VU) error {
		if err := checkWYTD(vu, vu.Conn()); err != nil {
			return err
		}
		log.Printf("[tpcc] check_consistency: OK")
		return nil
	})
}

// checkWYTD verifies TPC-C consistency condition 1: for every warehouse,
// w_ytd equals the sum of its districts' d_ytd (to 2 decimals).
func checkWYTD(vu *bench.VU, conn driver.Conn) error {
	bad, err := scalarInt(vu, conn, `
SELECT count(*)
FROM warehouse w
JOIN (SELECT d_w_id, sum(d_ytd) AS s FROM district GROUP BY d_w_id) d
  ON d.d_w_id = w.w_id
WHERE round(w.w_ytd::numeric, 2) <> round(d.s::numeric, 2)`)
	if err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("consistency: w_ytd != sum(d_ytd) for %d warehouses", bad)
	}
	return nil
}

// report prints the per-transaction tally and the New-Order throughput (tpmC-style)
// after the workload completes. Per-transaction latency histograms are not
// available (see the SDK-gap note in the package doc), so this is a count-only
// summary plus the built-in servicetime histogram from the reporter.
func report(o *options) bench.Handler {
	return bench.FuncOnce(func(vu *bench.VU) error {
		no := stats.newOrder.Load()
		total := no + stats.payment.Load() + stats.orderStatus.Load() +
			stats.delivery.Load() + stats.stockLevel.Load()
		mins := o.Duration.Seconds() / 60.0
		var tpmc float64
		if mins > 0 {
			tpmc = float64(no) / mins
		}
		fmt.Printf("\n=== tpcc transaction mix (W=%d, VUs=%d, %s) ===\n",
			o.Warehouses, o.VUs, o.Duration)
		fmt.Printf("  new_order    %8d  (rollbacks %d, remote lines %d)\n",
			no, stats.rollback.Load(), stats.remoteLine.Load())
		fmt.Printf("  payment      %8d  (remote %d, by-name %d, bc %d)\n",
			stats.payment.Load(), stats.remotePayment.Load(),
			stats.bynamePayment.Load(), stats.bcPayment.Load())
		fmt.Printf("  order_status %8d\n", stats.orderStatus.Load())
		fmt.Printf("  delivery     %8d\n", stats.delivery.Load())
		fmt.Printf("  stock_level  %8d\n", stats.stockLevel.Load())
		fmt.Printf("  total        %8d\n", total)
		fmt.Printf("  tpmC (new_order/min) = %.1f\n", tpmc)
		return nil
	})
}
