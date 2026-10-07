package rowmap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
