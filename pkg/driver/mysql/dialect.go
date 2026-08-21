package mysql

import (
	"errors"
	"fmt"
	"math"
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

// statementTimeoutGrace pads the client-side deadline past the server-side
// MAX_EXECUTION_TIME hint. Without it the client context timer fires one
// round-trip earlier than the server hint, so go-sql-driver/mysql cancels and
// discards the connection instead of letting the hint return its own 3024
// error. The hint fires at `timeout` (server-side); the padded client deadline
// is only a backstop.
const statementTimeoutGrace = time.Second

// StatementTimeoutHint bounds SELECT statements server-side with the
// MAX_EXECUTION_TIME optimizer hint so a timed-out query aborts cleanly and
// keeps its pooled connection, unlike client-side cancellation which forces
// go-sql-driver/mysql to discard the connection. The hint is recognized only
// when the statement's first token is SELECT: WITH/EXPLAIN-prefixed or
// comment-prefixed statements rely on the client deadline backstop, and
// non-SELECT statements (including INSERT ... SELECT) are intentionally left
// untouched because the hint does not bind them.
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

// StatementDeadline returns the client-side deadline padded past the
// server-side hint so the hint's 3024 timeout wins over client cancellation.
func (mysqlDialect) StatementDeadline(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return timeout
	}

	if timeout > time.Duration(math.MaxInt64)-statementTimeoutGrace {
		return time.Duration(math.MaxInt64)
	}

	return timeout + statementTimeoutGrace
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
