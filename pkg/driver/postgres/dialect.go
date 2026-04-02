package postgres

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	sqlqueries "github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

var _ sqlqueries.Dialect = PgxDialect{}

// ErrUnsupportedType is returned when a proto Value has an unrecognized type.
var ErrUnsupportedType = errors.New("unsupported value type")

// PgxDialect implements sqlqueries.Dialect for PostgreSQL via pgx.
type PgxDialect struct{}

func (PgxDialect) Placeholder(index int) string {
	return fmt.Sprintf("$%d", index+1)
}

func (PgxDialect) Convert(val any) (any, error) {
	switch v := val.(type) {
	case nil:
		return nil, nil //nolint:nilnil // allow to set nil in db
	case uuid.UUID:
		return &pgtype.UUID{Valid: true, Bytes: v}, nil
	case time.Time:
		return &pgtype.Timestamptz{Valid: true, Time: v}, nil
	case decimal.Decimal:
		return pgxdecimal.Decimal(v), nil
	default:
		return v, nil
	}
}

func (PgxDialect) Deduplicate() bool { return true }
