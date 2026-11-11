package dsdgen

import "testing"

func TestReasonByteEqual(t *testing.T) {
	assertTableByteEqual(t, Reason, 1, 10, 100)
}

func TestReturnReasonsStayWithinSpecification(t *testing.T) {
	for _, table := range []*FactTable{StoreReturns, CatalogReturns, WebReturns} {
		column := -1
		for i, name := range table.Columns {
			if name == "sr_reason_sk" || name == "cr_reason_sk" || name == "wr_reason_sk" {
				column = i
			}
		}
		if column < 0 {
			t.Fatal("return reason column missing")
		}
		stream := table.NewStream(1, 1, 100)
		for row, ok := stream.Next(); ok; row, ok = stream.Next() {
			if row[column] != nil && (row[column].(int64) < 1 || row[column].(int64) > 35) {
				t.Fatalf("%s reason key %v outside specification range", table.Name, row[column])
			}
		}
	}
}

func TestReasonIntermediateScaling(t *testing.T) {
	for _, test := range []struct {
		scale float64
		want  int64
	}{{0.01, 1}, {1, 35}, {5.5, 40}, {10, 45}, {50, 49}, {100, 55}, {101, 55}, {200, 57}, {300, 60}, {650, 62}, {1000, 65}} {
		if got := Reason.RowCount(test.scale); got != test.want {
			t.Errorf("reason SF=%g: got %d, want %d", test.scale, got, test.want)
		}
	}
	var previous int64
	for scale := 1; scale <= 100000; scale++ {
		got := Reason.RowCount(float64(scale))
		if got < previous || got > 75 {
			t.Fatalf("reason cardinality decreased or exceeded 75 at SF=%d: %d after %d", scale, got, previous)
		}
		previous = got
	}
}

func TestReasonSpecificationCardinality(t *testing.T) {
	for i, scale := range definedScales[1:] {
		want := []int64{35, 45, 55, 60, 65, 67, 70, 72, 75}[i]
		if got := Reason.RowCount(scale); got != want {
			t.Errorf("reason SF=%g: got %d rows, want %d", scale, got, want)
		}
		stream := NewRNStream(300, 16)
		for range 100 {
			key := GenerateJoinKey(TStoreReturns, JCNone, stream, TReason, 1, NewScaling(scale))
			if key < 1 || key > want {
				t.Fatalf("reason SF=%g: foreign key %d outside [1,%d]", scale, key, want)
			}
		}
	}
}
