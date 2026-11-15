package gen

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkMaterializeRowBytes(b *testing.B) {
	builder := NewSchemaBuilder()
	text := builder.Bytes("text", 32)
	batch := NewBatch(builder.Build(), 1)

	data, err := batch.AddRow().Bytes(text, 16)
	if err != nil {
		b.Fatal(err)
	}

	copy(data, "0123456789abcdef")

	for _, borrow := range []bool{false, true} {
		b.Run(strconv.FormatBool(borrow), func(b *testing.B) {
			out := make([]any, 1)

			var (
				cell    []byte
				convert func(int, []byte) any
			)
			if borrow {
				convert = func(_ int, data []byte) any {
					cell = data

					return &cell
				}
			}

			b.ReportAllocs()

			for b.Loop() {
				batch.MaterializeRowWithBytes(0, out, convert)
			}
		})
	}
}

func TestMaterializeRowBorrowedBytes(t *testing.T) {
	t.Parallel()

	builder := NewSchemaBuilder()
	text := builder.Bytes("text", 8)
	missing := builder.Bytes("missing", 8)
	batch := NewBatch(builder.Build(), 1)
	row := batch.AddRow()
	data, err := row.Bytes(text, 3)
	require.NoError(t, err)
	copy(data, "one")
	row.SetNull(missing)

	var borrowed []byte

	calls := 0
	out := make([]any, 2)
	batch.MaterializeRowWithBytes(0, out, func(column int, data []byte) any {
		require.Zero(t, column)

		borrowed = data
		calls++

		return &borrowed
	})
	require.Equal(t, 1, calls)
	require.Nil(t, out[1])
	require.Equal(t, []byte("one"), borrowed)

	owned := make([]any, 2)
	batch.MaterializeRow(0, owned)
	batch.Reset()
	row = batch.AddRow()
	data, err = row.Bytes(text, 3)
	require.NoError(t, err)
	copy(data, "two")
	require.Equal(t, []byte("two"), borrowed)
	require.Equal(t, "one", owned[0])
}
