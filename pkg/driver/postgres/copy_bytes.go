package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stroppy-io/stroppy/v6/pkg/driver/common"
)

var errCopyBytesType = errors.New("COPY byte encoder received an unsupported value")

type copyBytes struct{ data []byte }

func (v *copyBytes) TextValue() (pgtype.Text, error) {
	return pgtype.Text{String: string(v.data), Valid: true}, nil
}

func (v *copyBytes) Int64Value() (pgtype.Int8, error) {
	return copyText(v.data).Int64Value()
}

func (v *copyBytes) NumericValue() (pgtype.Numeric, error) {
	return copyText(v.data).NumericValue()
}

type copyBytesCodec struct{ pgtype.Codec }

func (c copyBytesCodec) PlanEncode(m *pgtype.Map, oid uint32, format int16, value any) pgtype.EncodePlan {
	if _, ok := value.(*copyBytes); ok && format == pgtype.BinaryFormatCode {
		return copyBytesPlan{}
	}

	return c.Codec.PlanEncode(m, oid, format, value)
}

type copyBytesPlan struct{}

func (copyBytesPlan) Encode(value any, buf []byte) ([]byte, error) {
	cell, ok := value.(*copyBytes)
	if !ok {
		return nil, errCopyBytesType
	}

	return append(buf, cell.data...), nil
}

func borrowCopyBytes(src *common.BatchRowSource, fields []pgconn.FieldDescription, typeMap *pgtype.Map) func() {
	cells := make([]copyBytes, len(fields))
	borrow := make([]bool, len(fields))
	original := make(map[uint32]*pgtype.Type)

	for i, field := range fields {
		switch field.DataTypeOID {
		case pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID, pgtype.NumericOID:
			borrow[i] = true
		case pgtype.TextOID, pgtype.VarcharOID, pgtype.BPCharOID:
			if _, exists := original[field.DataTypeOID]; exists {
				borrow[i] = true

				continue
			}

			typ, ok := typeMap.TypeForOID(field.DataTypeOID)
			if !ok {
				continue
			}

			if _, plainText := typ.Codec.(pgtype.TextCodec); !plainText {
				continue
			}

			borrow[i] = true

			if _, exists := original[typ.OID]; !exists {
				original[typ.OID] = typ
				typeMap.RegisterType(&pgtype.Type{Name: typ.Name, OID: typ.OID, Codec: copyBytesCodec{Codec: typ.Codec}})
			}
		}
	}

	src.BorrowBytes(func(column int, data []byte) any {
		if !borrow[column] {
			return string(data)
		}

		cells[column].data = data

		return &cells[column]
	})

	return func() {
		src.BorrowBytes(nil)

		for _, typ := range original {
			typeMap.RegisterType(typ)
		}
	}
}
