package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/driver/common"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
)

func TestBorrowCopyBytesMultipleTextColumns(t *testing.T) {
	t.Parallel()

	builder := gen.NewSchemaBuilder()
	first := builder.Bytes("first", 8)
	second := builder.Bytes("second", 8)
	schema := builder.Build()
	source := gen.NewIndexedSource(schema, gen.New(1), "borrow", 1, 1, func(row gen.Row, _ uint64) error {
		for _, column := range []gen.Column{first, second} {
			data, err := row.Bytes(column, 3)
			if err != nil {
				return err
			}

			copy(data, "abc")
		}

		return nil
	})
	cursor, err := source.Prepare(0, 1, 1)
	require.NoError(t, err)

	src := common.NewBatchRowSource(cursor, schema.ColumnNames(), 2)
	m := pgtype.NewMap()
	original, ok := m.TypeForOID(pgtype.TextOID)
	require.True(t, ok)

	fields := []pgconn.FieldDescription{{DataTypeOID: pgtype.TextOID}, {DataTypeOID: pgtype.TextOID}}
	release := borrowCopyBytes(src, fields, m)
	values, err := src.Next()
	require.NoError(t, err)

	for _, value := range values {
		require.IsType(t, (*copyBytes)(nil), value)
		encoded, err := m.Encode(pgtype.TextOID, pgtype.BinaryFormatCode, value, nil)
		require.NoError(t, err)
		require.Equal(t, "abc", string(encoded))
	}

	release()

	restored, ok := m.TypeForOID(pgtype.TextOID)
	require.True(t, ok)
	require.Same(t, original, restored)
}
