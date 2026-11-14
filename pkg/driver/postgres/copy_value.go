package postgres

import (
	"strconv"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// pgx chooses the interface matching the destination column, so numeric text
// avoids COPY's failed-encode fallback without interpreting text columns as numbers.
type copyText string

func (v copyText) TextValue() (pgtype.Text, error) {
	return pgtype.Text{String: string(v), Valid: true}, nil
}

func (v copyText) Int64Value() (pgtype.Int8, error) {
	n, err := strconv.ParseInt(string(v), 10, 64)

	return pgtype.Int8{Int64: n, Valid: true}, err
}

func (v copyText) NumericValue() (pgtype.Numeric, error) {
	n, err := decimal.NewFromString(string(v))
	if err != nil {
		return pgtype.Numeric{}, err
	}

	return pgxdecimal.Decimal(n).NumericValue()
}
