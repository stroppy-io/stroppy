package picodata

import (
	"testing"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestNumericTextPreservesExactDecimal(t *testing.T) {
	types := pgtype.NewMap()
	pgxdecimal.Register(types)
	integerFormat := types.FormatCodeForOID(pgtype.Int8OID)
	preferNumericText(types)
	require.Equal(t, int16(pgtype.TextFormatCode), types.FormatCodeForOID(pgtype.NumericOID))
	require.Equal(t, int16(pgtype.TextFormatCode), types.FormatCodeForOID(pgtype.NumericArrayOID))
	require.Equal(t, integerFormat, types.FormatCodeForOID(pgtype.Int8OID))
	numeric, ok := types.TypeForOID(pgtype.NumericOID)
	require.True(t, ok)

	for _, text := range []string{"0", "0.00", "-123456789.1234567890123456789"} {
		value, err := numeric.Codec.DecodeValue(types, pgtype.NumericOID, pgtype.TextFormatCode, []byte(text))
		require.NoError(t, err)

		got, isDecimal := value.(decimal.Decimal)
		require.True(t, isDecimal)

		want, err := decimal.NewFromString(text)
		require.NoError(t, err)
		require.True(t, got.Equal(want))
	}
}
