package tpchgen_test

import (
	"errors"
	"io"
	"math"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/tpchgen"
)

// TestOfficialAnswersQ1Q6SF1 validates gotpc data WITHOUT a database by
// computing the TPC-H Q1 and Q6 aggregates directly in Go over the generated
// SF=1 lineitem rows and comparing to the official reference answers
// (workloads/tpch/answers_sf1.json) within the same ±1% tolerance the DB-side
// validate_answers step uses. If gotpc is dbgen-faithful these must match.
func TestOfficialAnswersQ1Q6SF1(t *testing.T) {
	if testing.Short() {
		t.Skip("SF=1 generation (~6M rows); skipped under -short")
	}

	g, err := tpchgen.New("lineitem", 1.0)
	if err != nil {
		t.Fatal(err)
	}

	src, err := g.Partition(0, -1)
	if err != nil {
		t.Fatal(err)
	}

	type agg struct {
		sumQty, sumBase, sumDisc, sumCharge, sumDiscount float64
		cnt                                              int64
	}

	groups := map[string]*agg{}

	var q6 float64

	for {
		row, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatal(err)
		}

		qty := float64(row[4].(int64))
		eprice := row[5].(float64)
		disc := row[6].(float64)
		tax := row[7].(float64)
		rflag := row[8].(string)
		lstatus := row[9].(string)
		ship := row[10].(string)

		// Q6: revenue over a shipdate year, discount 0.06±0.01, qty < 24.
		if ship >= "1994-01-01" && ship < "1995-01-01" &&
			disc >= 0.05-1e-9 && disc <= 0.07+1e-9 && qty < 24 {
			q6 += eprice * disc
		}

		// Q1: pricing summary, shipdate <= 1998-12-01 minus 90 days.
		if ship <= "1998-09-02" {
			k := rflag + lstatus

			a := groups[k]
			if a == nil {
				a = &agg{}
				groups[k] = a
			}

			a.sumQty += qty
			a.sumBase += eprice
			a.sumDisc += eprice * (1 - disc)
			a.sumCharge += eprice * (1 - disc) * (1 + tax)
			a.sumDiscount += disc
			a.cnt++
		}
	}

	within := func(name string, got, want float64) {
		t.Helper()

		den := math.Max(math.Abs(want), 1)
		if rel := math.Abs(got-want) / den; rel > 0.01 {
			t.Errorf("%s: got %.2f want %.2f (%.3f%% > 1%%)", name, got, want, rel*100)
		}
	}

	// Official SF=1 Q6.
	within("q6.revenue", q6, 123141078.23)

	// Official SF=1 Q1 (returnflag,linestatus -> sums).
	want := map[string]agg{
		"AF": {sumQty: 37734107, sumBase: 56586554400.73, sumDisc: 53758257134.87, sumCharge: 55909065222.83, cnt: 1478493},
		"NF": {sumQty: 991417, sumBase: 1487504710.38, sumDisc: 1413082168.05, sumCharge: 1469649223.19, cnt: 38854},
		"NO": {
			sumQty: 74476040, sumBase: 111701729697.74,
			sumDisc: 106118230307.61, sumCharge: 110367043872.50, cnt: 2920374,
		},
		"RF": {sumQty: 37719753, sumBase: 56568041380.90, sumDisc: 53741292684.60, sumCharge: 55889619119.83, cnt: 1478870},
	}

	for k, w := range want {
		a := groups[k]
		if a == nil {
			t.Fatalf("Q1 group %q missing", k)

			continue
		}

		within("q1["+k+"].sum_qty", a.sumQty, w.sumQty)
		within("q1["+k+"].sum_base_price", a.sumBase, w.sumBase)
		within("q1["+k+"].sum_disc_price", a.sumDisc, w.sumDisc)
		within("q1["+k+"].sum_charge", a.sumCharge, w.sumCharge)
		within("q1["+k+"].count", float64(a.cnt), float64(w.cnt))

		avgDisc := a.sumDiscount / float64(a.cnt)
		if avgDisc < 0.04 || avgDisc > 0.06 {
			t.Errorf("q1[%s].avg_disc = %.4f, want ~0.05", k, avgDisc)
		}
	}
}
