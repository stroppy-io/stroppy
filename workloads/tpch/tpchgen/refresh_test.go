package tpchgen

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"testing"

	"github.com/stroppy-io/stroppy/v6/workloads/tpch/dbgen"
)

// These digests come from unmodified C DBGEN -s 1 -U 2, electrum/tpch-dbgen
// revision 32f1c1b92d1664dba542e927d23d86ffa57aa253, not this generator's output.
// DBGEN 2.17.3 (gregrahn/tpch-kit 852ad0a5ee31ebefeed884cea4188781dd9613a3)
// matches the same rows after normalizing its omitted trailing delimiter.
func TestRefreshMatchesDBGEN(t *testing.T) {
	golden := [][3]string{
		{
			"9959cc8fc853d82cc8a424828af2ea4c3a9e9b36f3ec4b0fece52f130a26a8aa",
			"bd924b1d723f906ca63f8b76e8dd19deb7c55374cbb0cc47785c73bb90cdea1f",
			"801d3d83fd0735a07ac9c60c247d93f2233bdd17d1e5ff7a7e9c81629c6cb8b9",
		},
		{
			"eaee073d331dd1bf9d1d00dc88c8d58fef514c23f760f23099b24d568383ef28",
			"0f39c9184a43da4bde2b7f5190159618a3819009fb23fd1bc87fe2190f5ea833",
			"2e17020ddd0807cbe26c7057e774cb20782265387893b337825c307e369c0f93",
		},
	}
	for index, want := range golden {
		generator, err := NewRefreshGenerator(1, int64(index+1))
		if err != nil {
			t.Fatal(err)
		}

		orders, lines, deletes := sha256.New(), sha256.New(), sha256.New()
		for i := range generator.Count() {
			fmt.Fprintf(deletes, "%d|\n", generator.DeleteKey(i))
		}

		count := 0

		for {
			order, ok := generator.Next()
			if !ok {
				break
			}

			writeRefreshOrder(orders, lines, order)

			count++
		}

		if count != 1500 {
			t.Fatalf("orders=%d", count)
		}

		for i, hash := range []hash.Hash{orders, lines, deletes} {
			if got := hex.EncodeToString(hash.Sum(nil)); got != want[i] {
				t.Fatalf("set=%d file=%d hash=%s want=%s", index+1, i, got, want[i])
			}
		}
	}
}

func writeRefreshOrder(orders, lines io.Writer, order *dbgen.Order) {
	fmt.Fprintf(orders, "%d|%d|%s|%.2f|%s|%s|%s|%d|%s|\n", order.OKey, order.CustKey, order.Status,
		dbgen.Money(int64(order.TotalPrice)), order.Date, order.OrderPriority, order.Clerk, order.ShipPriority, order.Comment)

	for _, line := range order.Lines {
		fmt.Fprintf(lines, "%d|%d|%d|%d|%d|%.2f|%.2f|%.2f|%s|%s|%s|%s|%s|%s|%s|%s|\n",
			line.OKey, line.PartKey, line.SuppKey, line.LCnt, line.Quantity, dbgen.Money(int64(line.EPrice)),
			dbgen.Money(int64(line.Discount)), dbgen.Money(int64(line.Tax)), line.RFlag, line.LStatus,
			line.SDate, line.CDate, line.RDate, line.ShipInstruct, line.ShipMode, line.Comment)
	}
}

func TestRefreshSparseCycles(t *testing.T) {
	for _, set := range []int64{1, 999, 1000, 1001, 2000, 3000, 4000} {
		generator, err := NewRefreshGenerator(1, set)
		if err != nil {
			t.Fatal(err)
		}

		order, ok := generator.Next()
		if !ok {
			t.Fatal("empty update")
		}

		index := (set-1)*1500 + 1
		if int64(order.OKey) != dbgen.SparseKey(index, 1+set/1000) ||
			generator.DeleteKey(0) != dbgen.SparseKey(index, (set/1000)&3) {
			t.Fatalf("set %d sparse keys differ", set)
		}

		for _, line := range order.Lines {
			if line.OKey != order.OKey {
				t.Fatal("orphan refresh line")
			}
		}
	}
}
