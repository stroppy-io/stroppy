package mysql

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

var _ queries.Dialect = mysqlDialect{}

var ErrUnsupportedType = errors.New("unsupported value type")

type mysqlDialect struct{}

func (mysqlDialect) Placeholder(_ int) string { return "?" }
func (mysqlDialect) Deduplicate() bool        { return false }

// StatementTimeoutHint bounds SELECT statements server-side with the
// MAX_EXECUTION_TIME optimizer hint so a timed-out query aborts cleanly and
// keeps its pooled connection, unlike client-side cancellation which forces
// go-sql-driver/mysql to discard the connection. The hint is ignored by
// non-SELECT statements, so inserting it after the SELECT keyword only is
// safe for the DDL and DML that also flow through this dialect.
func (mysqlDialect) StatementTimeoutHint(sql string, timeout time.Duration) string {
	if timeout <= 0 {
		return sql
	}

	const keyword = "SELECT"

	trimmed := strings.TrimLeft(sql, " \t\r\n")

	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, keyword) {
		return sql
	}

	// Only inject when "SELECT" is a whole keyword, not a longer identifier.
	if rest := upper[len(keyword):]; rest != "" && !isSQLWhitespace(rest[0]) {
		return sql
	}

	ms := timeout.Milliseconds()
	if ms < 1 {
		ms = 1
	}

	lead := sql[:len(sql)-len(trimmed)]

	return lead + trimmed[:len(keyword)] + " " +
		fmt.Sprintf("/*+ MAX_EXECUTION_TIME(%d) */", ms) + trimmed[len(keyword):]
}

func isSQLWhitespace(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func (mysqlDialect) Convert(val any) (any, error) {
	switch v := val.(type) { //nolint:varnamelen // switch type assertion idiom
	case nil:
		return nil, nil //nolint:nilnil // allow to set nil in db
	case uuid.UUID:
		return v.String(), nil
	case time.Time:
		return v, nil
	case decimal.Decimal:
		return v.String(), nil
	case *decimal.Decimal:
		return v.String(), nil
	default:
		return v, nil
	}
}
