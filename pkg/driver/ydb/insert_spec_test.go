package ydb

import (
	"testing"
	"time"

	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"
)

func TestMergeColumnTypesCachesAcrossBatches(t *testing.T) {
	t.Parallel()

	colTypes := make([]types.Type, 2)

	mergeColumnTypes(colTypes, [][]any{
		{int64(1), nil},
	})
	if colTypes[0] != types.TypeInt64 {
		t.Fatalf("col0 = %v, want Int64", colTypes[0])
	}
	if colTypes[1] != nil {
		t.Fatalf("col1 = %v, want unknown", colTypes[1])
	}

	effective := effectiveColumnTypes(colTypes, [][]any{{int64(1), nil}})
	if effective[1] != types.TypeInt64 {
		t.Fatalf("effective col1 = %v, want Int64 fallback", effective[1])
	}

	mergeColumnTypes(colTypes, [][]any{
		{int64(2), "text"},
	})
	if colTypes[0] != types.TypeInt64 {
		t.Fatalf("col0 changed to %v", colTypes[0])
	}
	if colTypes[1] != types.TypeText {
		t.Fatalf("col1 = %v, want Text", colTypes[1])
	}
}

func TestColumnsWithNulls(t *testing.T) {
	t.Parallel()

	hasNull := columnsWithNulls(
		[]string{"a", "b"},
		[][]any{
			{int64(1), int64(2)},
			{nil, int64(3)},
		},
	)

	if !hasNull[0] || hasNull[1] {
		t.Fatalf("hasNull = %v, want [true false]", hasNull)
	}
}

func TestRowToStructValueTypedOptionalNull(t *testing.T) {
	t.Parallel()

	ts := time.Unix(1_700_000_000, 0).UTC()
	columns := []string{"id", "carrier", "delivered"}
	colTypes := []types.Type{types.TypeInt64, types.TypeInt64, types.TypeTimestamp}
	hasNull := []bool{false, true, true}
	fields := make([]types.StructValueOption, len(columns))

	sv, err := rowToStructValueTyped(columns, []any{
		int64(1),
		int64(7),
		&ts,
	}, colTypes, hasNull, fields)
	if err != nil {
		t.Fatalf("non-null row: %v", err)
	}
	if sv == nil {
		t.Fatal("expected struct value")
	}

	sv, err = rowToStructValueTyped(columns, []any{
		int64(2),
		nil,
		nil,
	}, colTypes, hasNull, fields)
	if err != nil {
		t.Fatalf("null row: %v", err)
	}
	if sv == nil {
		t.Fatal("expected struct value")
	}
}

func TestConvertRowInto(t *testing.T) {
	t.Parallel()

	columns := []string{"w_id", "w_tax"}
	dest := make([]any, 2)

	err := convertRowInto(ydbDialect{}, columns, dest, []any{int64(1), 0.05})
	if err != nil {
		t.Fatalf("convertRowInto: %v", err)
	}

	if dest[0] != int64(1) {
		t.Fatalf("dest[0] = %v", dest[0])
	}
	if dest[1] != 0.05 {
		t.Fatalf("dest[1] = %v", dest[1])
	}
}
