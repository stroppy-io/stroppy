package postgres

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/driver/common"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
)

func TestCopyBytesInt64Parity(t *testing.T) {
	t.Parallel()

	texts := []string{
		"", "0", "-0", "+0", "-000", "123", "-123", "9223372036854775807", "-9223372036854775808",
		"9223372036854775808", "1.5", " 1", "+", "0000000000000000000000001",
	}

	cases := make([]string, len(texts), len(texts)+10000)
	copy(cases, texts)
	texts = cases

	rng := rand.New(rand.NewPCG(30, 40))
	for range 10000 {
		texts = append(texts, strconv.FormatInt(rng.Int64(), 10))
	}

	for _, text := range texts {
		cell := copyBytes{data: []byte(text)}
		got, err := cell.Int64Value()

		want, wantErr := copyText(text).Int64Value()
		if wantErr != nil {
			require.EqualError(t, err, wantErr.Error(), text)
		} else {
			require.NoError(t, err, text)
			require.Equal(t, want, got, text)
		}
	}
}

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
