package picodata

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

var _ sqlqueries.Dialect = PicoDialect{}

// ErrUnsupportedType is returned when a proto Value has an unrecognized type.
var ErrUnsupportedType = errors.New("unsupported value type")

// PicoDialect implements sqlqueries.Dialect for Picodata via pgx.
type PicoDialect struct{}

var (
	picoPlaceholderCache []string
	picoPlaceholderMu    sync.RWMutex
)

func (PicoDialect) Placeholder(index int) string {
	picoPlaceholderMu.RLock()

	if index < len(picoPlaceholderCache) {
		s := picoPlaceholderCache[index]

		picoPlaceholderMu.RUnlock()

		return s
	}

	picoPlaceholderMu.RUnlock()

	picoPlaceholderMu.Lock()
	defer picoPlaceholderMu.Unlock()

	// Re-check after acquiring write lock.
	if index < len(picoPlaceholderCache) {
		return picoPlaceholderCache[index]
	}

	for i := len(picoPlaceholderCache); i <= index; i++ {
		picoPlaceholderCache = append(picoPlaceholderCache, fmt.Sprintf("$%d", i+1))
	}

	return picoPlaceholderCache[index]
}

func (PicoDialect) Convert(val any) (any, error) {
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

func (PicoDialect) Deduplicate() bool { return true }
