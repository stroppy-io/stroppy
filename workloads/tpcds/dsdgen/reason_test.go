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
