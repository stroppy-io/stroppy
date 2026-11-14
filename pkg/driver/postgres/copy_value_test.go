package postgres

import (
	"testing"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestCopyTextBinaryEncoding(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		oid  uint32
		text string
	}{
		{"int2", pgtype.Int2OID, "-32768"},
		{"int4", pgtype.Int4OID, "2147483647"},
		{"int8", pgtype.Int8OID, "9223372036854775807"},
		{"numeric", pgtype.NumericOID, "-12345678901234567890.0012300"},
		{"zero", pgtype.NumericOID, "0.00"},
		{"text", pgtype.TextOID, "000123.00"},
		{"varchar", pgtype.VarcharOID, "000123"},
		{"empty", pgtype.TextOID, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			m := pgtype.NewMap()
			pgxdecimal.Register(m)

			value := copyText(test.text)
			require.NotNil(t, m.PlanEncode(test.oid, pgtype.BinaryFormatCode, value))
			got, err := m.Encode(test.oid, pgtype.BinaryFormatCode, value, nil)
			require.NoError(t, err)

			var expected any
			require.NoError(t, m.Scan(test.oid, pgtype.TextFormatCode, []byte(test.text), &expected))
			want, err := m.Encode(test.oid, pgtype.BinaryFormatCode, expected, nil)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func TestCopyTextNegativeZeroFallback(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"-0", "-000"} {
		value := copyText(text)
		_, err := value.Int64Value()
		require.ErrorIs(t, err, errCopyNegativeZero)

		for _, oid := range []uint32{pgtype.Float4OID, pgtype.Float8OID} {
			m := pgtype.NewMap()
			_, err := m.Encode(oid, pgtype.BinaryFormatCode, value, nil)
			require.ErrorIs(t, err, errCopyNegativeZero)
			buf, err := m.Encode(oid, pgtype.TextFormatCode, value, nil)
			require.NoError(t, err)
			require.Equal(t, text, string(buf))
		}
	}
}

func TestCopyRowSourceValues(t *testing.T) {
	t.Parallel()

	row := []any{"000123", int64(42), nil, []byte("bytes")}
	src := &rowSource{
		src:            &fakeSource{cols: []string{"text", "number", "null", "bytes"}, rows: [][]any{row}},
		columnCount:    len(row),
		numericColumns: []int{0, 1, 2},
	}
	require.True(t, src.Next())
	values, err := src.Values()
	require.NoError(t, err)
	require.Equal(t, []any{copyText("000123"), int64(42), nil, []byte("bytes")}, values)

	again, err := src.Values()
	require.NoError(t, err)
	require.Equal(t, values, again)
	require.False(t, src.Next())
	require.NoError(t, src.Err())
}

func TestCopyNumericColumns(t *testing.T) {
	t.Parallel()

	fields := []pgconn.FieldDescription{
		{DataTypeOID: pgtype.TextOID},
		{DataTypeOID: pgtype.Int2OID},
		{DataTypeOID: pgtype.Int4OID},
		{DataTypeOID: pgtype.Int8OID},
		{DataTypeOID: pgtype.NumericOID},
		{DataTypeOID: pgtype.Float4OID},
		{DataTypeOID: pgtype.Float8OID},
		{DataTypeOID: pgtype.VarcharOID},
	}
	require.Equal(t, []int{1, 2, 3, 4}, copyNumericColumns(fields))

	row := []any{"00123", "123", int64(42), nil, "1.25", "-0", "-000", "0001"}
	src := &rowSource{row: row, columnCount: len(fields), numericColumns: copyNumericColumns(fields)}
	values, err := src.Values()
	require.NoError(t, err)
	require.Equal(t, []any{"00123", copyText("123"), int64(42), nil, copyText("1.25"), "-0", "-000", "0001"}, values)

	src.row = []any{"short"}
	values, err = src.Values()
	require.NoError(t, err)
	require.Equal(t, []any{"short"}, values)
}

func TestCopyTextInvalidValues(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		oid  uint32
		text string
	}{
		{pgtype.Int2OID, "32768"},
		{pgtype.Int4OID, "2147483648"},
		{pgtype.Int8OID, "9223372036854775808"},
		{pgtype.Int4OID, "1.5"},
		{pgtype.Int4OID, ""},
		{pgtype.NumericOID, "not numeric"},
	} {
		m := pgtype.NewMap()
		pgxdecimal.Register(m)
		_, err := m.Encode(test.oid, pgtype.BinaryFormatCode, copyText(test.text), nil)
		require.Error(t, err)
	}
}
