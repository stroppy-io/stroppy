package mysql

import (
	"errors"
	"fmt"
	"math"
	"regexp"
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
const (
	statementTimeoutGrace = time.Second
	maxStatementTimeout   = time.Duration(math.MaxUint32) * time.Millisecond
)

var maxExecutionTimeHintPattern = regexp.MustCompile(
	`(?i)\bMAX_EXECUTION_TIME[ \t\r\n]*\([ \t\r\n]*[0-9]+[ \t\r\n]*\)`,
)

// StatementTimeoutHint bounds eligible SELECT statements server-side with the
// MAX_EXECUTION_TIME optimizer hint so a timed-out query aborts cleanly and
// keeps its pooled connection, unlike client-side cancellation which forces
// go-sql-driver/mysql to discard the connection. Existing optimizer hints are
// kept in the single hint block MySQL recognizes after SELECT. WITH/EXPLAIN-
// prefixed, comment-prefixed, non-SELECT, and top-level SELECT SLEEP statements
// rely on the client deadline backstop.
func (mysqlDialect) StatementTimeoutHint(sql string, timeout time.Duration) string {
	milliseconds, ok := mysqlTimeoutMilliseconds(timeout)
	if !ok {
		return sql
	}

	selectEnd, ok := selectKeywordEnd(sql)
	if !ok {
		return sql
	}

	hintStart, hintEnd, hasHint := optimizerHintBounds(sql, selectEnd)

	expressionStart := selectEnd
	if hasHint {
		expressionStart = hintEnd
	}

	if isTopLevelSleep(sql[expressionStart:]) {
		return sql
	}

	timeoutHint := fmt.Sprintf("MAX_EXECUTION_TIME(%d)", milliseconds)
	if !hasHint {
		return sql[:selectEnd] + " /*+ " + timeoutHint + " */" + sql[selectEnd:]
	}

	contentStart := hintStart + len("/*+")
	contentEnd := hintEnd - len("*/")
	content := sql[contentStart:contentEnd]

	replaced, found := replaceMaxExecutionTimeHints(content, timeoutHint)
	if found {
		content = replaced
	} else {
		content = insertOptimizerHint(content, timeoutHint)
	}

	return sql[:contentStart] + content + sql[contentEnd:]
}

func replaceMaxExecutionTimeHints(content, hint string) (string, bool) {
	matches := maxExecutionTimeHintPattern.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return content, false
	}

	var replaced strings.Builder
	replaced.Grow(len(content) - matches[0][1] + matches[0][0] + len(hint))

	last := 0
	for index, match := range matches {
		replaced.WriteString(content[last:match[0]])

		if index == 0 {
			replaced.WriteString(hint)
		}

		last = match[1]
	}

	replaced.WriteString(content[last:])

	return replaced.String(), true
}

// StatementDeadline pads the client-side deadline only for durations that can
// be represented by a MAX_EXECUTION_TIME hint.
func (mysqlDialect) StatementDeadline(timeout time.Duration) time.Duration {
	if _, ok := mysqlTimeoutMilliseconds(timeout); !ok {
		return timeout
	}

	return timeout + statementTimeoutGrace
}

func mysqlTimeoutMilliseconds(timeout time.Duration) (uint32, bool) {
	if timeout < time.Millisecond || timeout > maxStatementTimeout {
		return 0, false
	}

	return uint32(timeout / time.Millisecond), true
}

func selectKeywordEnd(sql string) (int, bool) {
	const keyword = "SELECT"

	start := skipSQLWhitespace(sql, 0)
	end := start + len(keyword)

	if end > len(sql) || !strings.EqualFold(sql[start:end], keyword) {
		return 0, false
	}

	if end < len(sql) && isSQLIdentifierByte(sql[end]) {
		return 0, false
	}

	return end, true
}

func optimizerHintBounds(sql string, offset int) (hintStart, hintEnd int, found bool) {
	start := skipSQLWhitespace(sql, offset)
	if !strings.HasPrefix(sql[start:], "/*+") {
		return 0, 0, false
	}

	closeOffset := strings.Index(sql[start+len("/*+"):], "*/")
	if closeOffset < 0 {
		return 0, 0, false
	}

	end := start + len("/*+") + closeOffset + len("*/")

	return start, end, true
}

func insertOptimizerHint(content, hint string) string {
	insertAt := len(content)
	for insertAt > 0 && isSQLWhitespace(content[insertAt-1]) {
		insertAt--
	}

	separator := ""
	if insertAt > 0 {
		separator = " "
	}

	return content[:insertAt] + separator + hint + content[insertAt:]
}

func isTopLevelSleep(sql string) bool {
	start := skipLeadingBlockComments(sql)

	const function = "SLEEP"

	end := start + len(function)
	if end > len(sql) || !strings.EqualFold(sql[start:end], function) {
		return false
	}

	if end < len(sql) && isSQLIdentifierByte(sql[end]) {
		return false
	}

	end = skipSQLWhitespace(sql, end)

	return end < len(sql) && sql[end] == '('
}

func skipLeadingBlockComments(sql string) int {
	offset := skipSQLWhitespace(sql, 0)
	for strings.HasPrefix(sql[offset:], "/*") {
		closeOffset := strings.Index(sql[offset+len("/*"):], "*/")
		if closeOffset < 0 {
			return offset
		}

		offset += len("/*") + closeOffset + len("*/")
		offset = skipSQLWhitespace(sql, offset)
	}

	return offset
}

func skipSQLWhitespace(sql string, offset int) int {
	for offset < len(sql) && isSQLWhitespace(sql[offset]) {
		offset++
	}

	return offset
}

func isSQLIdentifierByte(char byte) bool {
	return char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' ||
		char == '_' || char == '$' || char >= 0x80
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
