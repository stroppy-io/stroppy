package postgres

import (
	"testing"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
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
	src := &rowSource{src: &fakeSource{cols: []string{"text", "number", "null", "bytes"}, rows: [][]any{row}}}
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
