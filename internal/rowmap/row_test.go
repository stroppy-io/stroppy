package rowmap

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

type formattedInteger int64

func (formattedInteger) String() string { return "123" }

func TestAssignScalarMatchesTextConversion(t *testing.T) {
	for _, value := range []any{
		int(-7), int8(-8), int16(-16), int32(-32), int64(123456789),
		uint(7), uint8(8), uint16(16), uint32(32), uint64(math.MaxUint64), uintptr(42),
		float32(1.2345678), float64(1.2345678901234567), math.Inf(1), math.NaN(),
		true, false, "42", []byte("42"), formattedInteger(7),
	} {
		for _, destination := range []any{
			int(0), int8(0), int16(0), int32(0), int64(0),
			uint(0), uint8(0), uint16(0), uint32(0), uint64(0), uintptr(0),
			float32(0), float64(0), false, "",
		} {
			t.Run(fmt.Sprintf("%T=%v/to=%T", value, value, destination), func(t *testing.T) {
				got := reflect.New(reflect.TypeOf(destination)).Elem()
				want := reflect.New(got.Type()).Elem()

				text := fmt.Sprint(value)
				if bytes, ok := value.([]byte); ok {
					text = string(bytes)
				}

				wantErr := Assign(want, text)

				gotErr := Assign(got, value)
				if wantErr != nil {
					require.EqualError(t, gotErr, wantErr.Error())

					return
				}

				require.NoError(t, gotErr)

				if got.CanFloat() && math.IsNaN(want.Float()) {
					require.True(t, math.IsNaN(got.Float()))
				} else {
					require.Equal(t, want.Interface(), got.Interface())
				}
			})
		}
	}
}

func TestDecodeFormattedSourceRetainsMethods(t *testing.T) {
	value, err := Decode[formattedInteger](nil, []any{formattedInteger(7)})
	require.NoError(t, err)
	require.Equal(t, formattedInteger(123), value)
}

func TestDecodeExactScalarAllocations(t *testing.T) {
	values := []any{int64(42)}
	allocations := testing.AllocsPerRun(1000, func() {
		value, err := Decode[int64](nil, values)
		if err != nil || value != 42 {
			t.Fatalf("Decode() = %v, %v", value, err)
		}
	})
	require.Zero(t, allocations)
}

func TestAssignOverflowKeepsErrorAndDestination(t *testing.T) {
	value := int8(7)
	err := Assign(reflect.ValueOf(&value).Elem(), int64(256))

	var numberError *strconv.NumError
	require.ErrorAs(t, err, &numberError)
	require.ErrorIs(t, err, strconv.ErrRange)
	require.Equal(t, int8(7), value)
}

func TestDecodeScalarAndStruct(t *testing.T) {
	value, err := Decode[int64]([]string{"value"}, []any{int64(42)})
	require.NoError(t, err)
	require.Equal(t, int64(42), value)

	text, err := Decode[string]([]string{"value"}, []any{[]byte("hello")})
	require.NoError(t, err)
	require.Equal(t, "hello", text)

	type account struct {
		ID    int64   `db:"id"`
		Label *string `db:"label"`
	}

	item, err := Decode[account]([]string{"label", "id"}, []any{nil, "7"})
	require.NoError(t, err)
	require.Equal(t, int64(7), item.ID)
	require.Nil(t, item.Label)
}

func TestDecodeRejectsNullShapeAndOverflow(t *testing.T) {
	_, err := Decode[int64]([]string{"value"}, []any{nil})
	require.Error(t, err)
	_, err = Decode[int8]([]string{"value"}, []any{int64(256)})
	require.Error(t, err)
	_, err = Decode[int64]([]string{"first", "second"}, []any{1, 2})
	require.Error(t, err)

	type account struct{ ID int64 }

	_, err = Decode[account]([]string{"other"}, []any{1})
	require.ErrorContains(t, err, "missing result column")
	_, err = Decode[account]([]string{"id", "ID"}, []any{1, 2})
	require.ErrorContains(t, err, "ambiguous")
}

func TestOwnedByteValues(t *testing.T) {
	bytes := []byte("hello")
	value, err := Decode[[]byte]([]string{"value"}, []any{bytes})
	require.NoError(t, err)

	bytes[0] = 'x'

	require.Equal(t, "hello", string(value))
}
