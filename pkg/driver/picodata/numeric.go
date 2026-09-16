package picodata

import (
	"github.com/jackc/pgx/v5/pgtype"
)

// Legacy Picodata encodes some numeric zero values with a zero digit and
// nonnegative exponent. pgx's binary trailing-zero reduction does not terminate
// for that representation. Prefer exact text decimals; retain the configured
// codec's value type and all other result formats.
type numericTextCodec struct {
	pgtype.Codec
}

func (numericTextCodec) PreferredFormat() int16 { return pgtype.TextFormatCode }

func preferNumericText(types *pgtype.Map) {
	numeric, ok := types.TypeForOID(pgtype.NumericOID)
	if !ok {
		return
	}

	textNumeric := &pgtype.Type{Name: numeric.Name, OID: numeric.OID, Codec: numericTextCodec{numeric.Codec}}
	types.RegisterType(textNumeric)
	types.RegisterType(&pgtype.Type{
		Name: "_numeric", OID: pgtype.NumericArrayOID,
		Codec: numericTextCodec{&pgtype.ArrayCodec{ElementType: textNumeric}},
	})
}
