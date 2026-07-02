package ydb

import (
	"errors"
	"reflect"
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

	err := convertRowInto(ydbDialect{}, columns, nil, dest, []any{int64(1), 0.05})
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

func TestConvertRowIntoKinds(t *testing.T) {
	t.Parallel()

	// Timestamp column: date string -> *time.Time. Double column: int64 ->
	// float64. Passthrough string column is left untouched.
	columns := []string{"l_orderkey", "l_shipdate", "l_quantity", "l_comment"}
	kinds := []colKind{kindPassthrough, kindTimestamp, kindDouble, kindPassthrough}
	dest := make([]any, 4)

	err := convertRowInto(ydbDialect{}, columns, kinds, dest,
		[]any{int64(7), "1996-01-02", int64(17), "not-a-date"})
	if err != nil {
		t.Fatalf("convertRowInto: %v", err)
	}

	if dest[0] != int64(7) {
		t.Fatalf("dest[0] = %v (%T), want int64 7", dest[0], dest[0])
	}

	ts, ok := dest[1].(*time.Time)
	if !ok {
		t.Fatalf("dest[1] type = %T, want *time.Time", dest[1])
	}

	if ts.Year() != 1996 || ts.Month() != time.January || ts.Day() != 2 {
		t.Fatalf("dest[1] = %v, want 1996-01-02", ts)
	}

	if dest[2] != float64(17) {
		t.Fatalf("dest[2] = %v (%T), want float64 17", dest[2], dest[2])
	}

	if dest[3] != "not-a-date" {
		t.Fatalf("dest[3] = %v, want string untouched", dest[3])
	}
}

// The TPC-DS generator emits every cell as text; Int64/Double columns must
// parse those strings (kindInt64 / kindDouble string arm), while Utf8 columns
// pass the string through unchanged.
func TestConvertRowIntoTpcdsTextCells(t *testing.T) {
	t.Parallel()

	columns := []string{"ss_item_sk", "ss_net_profit", "s_state", "ss_sold_date_sk"}
	kinds := []colKind{kindInt64, kindDouble, kindPassthrough, kindInt64}
	dest := make([]any, 4)

	err := convertRowInto(ydbDialect{}, columns, kinds, dest,
		[]any{"42", "-12.50", "TN", nil})
	if err != nil {
		t.Fatalf("convertRowInto: %v", err)
	}

	if dest[0] != int64(42) {
		t.Fatalf("dest[0] = %v (%T), want int64 42", dest[0], dest[0])
	}

	if dest[1] != float64(-12.5) {
		t.Fatalf("dest[1] = %v (%T), want float64 -12.5", dest[1], dest[1])
	}

	if dest[2] != "TN" {
		t.Fatalf("dest[2] = %v, want string untouched", dest[2])
	}

	if dest[3] != nil {
		t.Fatalf("dest[3] = %v, want nil passthrough", dest[3])
	}
}

func TestConvertRowIntoRejectsBadNumericText(t *testing.T) {
	t.Parallel()

	dest := make([]any, 1)
	if err := convertRowInto(ydbDialect{}, []string{"n"}, []colKind{kindInt64}, dest,
		[]any{"not-a-number"}); err == nil {
		t.Fatal("convertRowInto: want error on non-integer text, got nil")
	}
}

// An all-null column in a batch has nothing to infer from; the declared schema
// type must win over the Int64 fallback so a legitimately-empty Utf8 column
// (TPC-DS c_login) is not sent as Int64 and rejected by BulkUpsert.
func TestEffectiveColumnTypesDeclaredFallback(t *testing.T) {
	t.Parallel()

	cached := make([]types.Type, 2)
	declared := []types.Type{types.TypeInt64, types.TypeText}
	dest := make([]types.Type, 2)

	got := effectiveColumnTypesInto(dest, cached, declared, [][]any{{int64(1), nil}})

	if got[0] != types.TypeInt64 {
		t.Fatalf("col0 = %v, want Int64 (inferred)", got[0])
	}

	if got[1] != types.TypeText {
		t.Fatalf("col1 = %v, want Text (declared fallback for all-null)", got[1])
	}
}

func TestDialectConvertAnySlice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []any
		want any
	}{
		{
			name: "int64 list",
			in:   []any{int64(1), int64(2), int64(3)},
			want: []int64{1, 2, 3},
		},
		{
			name: "string list",
			in:   []any{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "empty list passes through",
			in:   []any{},
			want: []any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ydbDialect{}.Convert(tt.in)
			if err != nil {
				t.Fatalf("Convert() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Convert() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDialectConvertAnySliceMixedTypes(t *testing.T) {
	t.Parallel()

	_, err := ydbDialect{}.Convert([]any{int64(1), "2"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("expected ErrUnsupportedType, got %v", err)
	}
}
