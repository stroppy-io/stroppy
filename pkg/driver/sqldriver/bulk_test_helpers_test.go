package sqldriver

import (
	"context"
	"time"

	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

// mockExecer captures every ExecContext call so tests can inspect the SQL
// emitted by the bulk helper.
type mockExecer struct {
	calls []execCall
	fail  error
	stop  int // if > 0, return fail starting at call index `stop`
}

type execCall struct {
	sql  string
	args []any
}

func (m *mockExecer) ExecContext(_ context.Context, sqlStr string, args ...any) (int64, error) {
	m.calls = append(m.calls, execCall{sql: sqlStr, args: append([]any(nil), args...)})

	if m.fail != nil && len(m.calls) >= m.stop {
		return 0, m.fail
	}

	return int64(len(args)), nil
}

var _ ExecContext[int64] = (*mockExecer)(nil)

// qmark is a minimal Dialect: "?" placeholder, pass-through Convert.
type qmark struct{}

func (qmark) Placeholder(_ int) string   { return "?" }
func (qmark) Convert(v any) (any, error) { return v, nil } //nolint:nilnil // pass-through
func (qmark) Deduplicate() bool          { return false }

func (qmark) StatementTimeoutHint(sql string, _ time.Duration) (string, bool) {
	return sql, false
}
func (qmark) StatementDeadline(timeout time.Duration) time.Duration { return timeout }

var _ queries.Dialect = qmark{}
