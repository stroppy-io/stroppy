package postgres

import (
	"errors"
	"strconv"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// pgx chooses the interface matching the destination column, so numeric text
// avoids COPY's failed-encode fallback without interpreting text columns as numbers.
type copyText string

var errCopyNegativeZero = errors.New("negative zero requires destination-specific text conversion")

func (v copyText) TextValue() (pgtype.Text, error) {
	return pgtype.Text{String: string(v), Valid: true}, nil
}

func (v copyText) Int64Value() (pgtype.Int8, error) {
	n, err := strconv.ParseInt(string(v), 10, 64)
	// Float codecs also accept Int64Valuer; their text fallback preserves the sign.
	if err == nil && n == 0 && v[0] == '-' {
		return pgtype.Int8{}, errCopyNegativeZero
	}

	return pgtype.Int8{Int64: n, Valid: true}, err
}

func (v copyText) NumericValue() (pgtype.Numeric, error) {
	n, err := decimal.NewFromString(string(v))
	if err != nil {
		return pgtype.Numeric{}, err
	}

	return pgxdecimal.Decimal(n).NumericValue()
}
