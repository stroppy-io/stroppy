package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func TestValueToAny_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		val  *stroppy.Value
	}{
		{"null", &stroppy.Value{Type: &stroppy.Value_Null{}}},
		{"int32", &stroppy.Value{Type: &stroppy.Value_Int32{Int32: 42}}},
		{"uint32", &stroppy.Value{Type: &stroppy.Value_Uint32{Uint32: 42}}},
		{"int64", &stroppy.Value{Type: &stroppy.Value_Int64{Int64: 42}}},
		{"uint64", &stroppy.Value{Type: &stroppy.Value_Uint64{Uint64: 42}}},
		{"float", &stroppy.Value{Type: &stroppy.Value_Float{Float: 3.14}}},
		{"double", &stroppy.Value{Type: &stroppy.Value_Double{Double: 2.71}}},
		{"string", &stroppy.Value{Type: &stroppy.Value_String_{String_: "abc"}}},
		{"bool", &stroppy.Value{Type: &stroppy.Value_Bool{Bool: true}}},
		{
			"decimal",
			&stroppy.Value{Type: &stroppy.Value_Decimal{Decimal: &stroppy.Decimal{Value: "1.23"}}},
		},
		{
			"uuid",
			&stroppy.Value{Type: &stroppy.Value_Uuid{Uuid: &stroppy.Uuid{Value: uuid.NewString()}}},
		},
		{
			"datetime",
			&stroppy.Value{Type: &stroppy.Value_Datetime{
				Datetime: &stroppy.DateTime{Value: timestamppb.New(time.Now())},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValueToAny(tt.val)
			require.NoError(t, err)
		})
	}
}

func TestValueToAny_Unsupported(t *testing.T) {
	val := &stroppy.Value{Type: &stroppy.Value_Struct_{Struct: &stroppy.Value_Struct{}}}

	_, err := ValueToAny(val)
	require.Error(t, err)
}

func TestValueToAny_DecimalNil(t *testing.T) {
	val := &stroppy.Value{Type: &stroppy.Value_Decimal{Decimal: nil}}

	result, err := ValueToAny(val)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestValueToAny_DecimalInvalid(t *testing.T) {
	val := &stroppy.Value{Type: &stroppy.Value_Decimal{Decimal: &stroppy.Decimal{Value: "invalid"}}}

	_, err := ValueToAny(val)
	require.Error(t, err)
}

func TestValueToAny_UuidInvalid(t *testing.T) {
	val := &stroppy.Value{Type: &stroppy.Value_Uuid{Uuid: &stroppy.Uuid{Value: "invalid-uuid"}}}

	_, err := ValueToAny(val)
	require.Error(t, err)
}

func TestValueToAny_ReturnValues(t *testing.T) {
	int32Val := &stroppy.Value{Type: &stroppy.Value_Int32{Int32: 42}}
	result, err := ValueToAny(int32Val)
	require.NoError(t, err)
	require.NotNil(t, result)

	stringVal := &stroppy.Value{Type: &stroppy.Value_String_{String_: "test"}}
	result, err = ValueToAny(stringVal)
	require.NoError(t, err)
	require.NotNil(t, result)

	boolVal := &stroppy.Value{Type: &stroppy.Value_Bool{Bool: true}}
	result, err = ValueToAny(boolVal)
	require.NoError(t, err)
	require.NotNil(t, result)

	nullVal := &stroppy.Value{Type: &stroppy.Value_Null{}}
	result, err = ValueToAny(nullVal)
	require.NoError(t, err)
	require.Nil(t, result)
}
