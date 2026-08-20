package postgres

import (
	"errors"
	"fmt"
	"sync"
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

var (
	pgxPlaceholderCache []string
	pgxPlaceholderMu    sync.RWMutex
)

func (PgxDialect) Placeholder(index int) string {
	pgxPlaceholderMu.RLock()

	if index < len(pgxPlaceholderCache) {
		s := pgxPlaceholderCache[index]

		pgxPlaceholderMu.RUnlock()

		return s
	}

	pgxPlaceholderMu.RUnlock()

	pgxPlaceholderMu.Lock()
	defer pgxPlaceholderMu.Unlock()

	// Re-check after acquiring write lock.
	if index < len(pgxPlaceholderCache) {
		return pgxPlaceholderCache[index]
	}

	for i := len(pgxPlaceholderCache); i <= index; i++ {
		pgxPlaceholderCache = append(pgxPlaceholderCache, fmt.Sprintf("$%d", i+1))
	}

	return pgxPlaceholderCache[index]
}

func (PgxDialect) Convert(val any) (any, error) {
	switch v := val.(type) { //nolint:varnamelen // switch type assertion idiom
	case nil:
		return nil, nil //nolint:nilnil // allow to set nil in db
	case uuid.UUID:
		return &pgtype.UUID{Valid: true, Bytes: v}, nil
	case time.Time:
		return &pgtype.Timestamptz{Valid: true, Time: v}, nil
	case *time.Time:
		return &pgtype.Timestamptz{Valid: true, Time: *v}, nil
	case decimal.Decimal:
		return pgxdecimal.Decimal(v), nil
	case *decimal.Decimal:
		return pgxdecimal.Decimal(*v), nil
	default:
		return v, nil
	}
}

func (PgxDialect) Deduplicate() bool { return true }

// StatementTimeoutHint returns sql unchanged: pgx already turns a deadline
// into a PostgreSQL CancelRequest, so no session-level statement_timeout (and
// no pooled-connection state to leak) is needed.
func (PgxDialect) StatementTimeoutHint(sql string, _ time.Duration) string { return sql }

// StatementDeadline returns timeout unchanged; there is no server-side hint
// to outlast, so the client deadline is the timeout itself.
func (PgxDialect) StatementDeadline(timeout time.Duration) time.Duration { return timeout }
