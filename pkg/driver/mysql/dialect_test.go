package mysql

import (
	"testing"
	"time"
)

func TestStatementTimeoutHint(t *testing.T) {
	t.Parallel()

	dynamic := mysqlDialect{}

	tests := []struct {
		name    string
		sql     string
		timeout time.Duration
		want    string
	}{
		{
			name:    "select gets hint after keyword",
			sql:     "SELECT 1",
			timeout: time.Second,
			want:    "SELECT /*+ MAX_EXECUTION_TIME(1000) */ 1",
		},
		{
			name:    "lowercase select preserves its keyword case",
			sql:     "select * from t",
			timeout: 2 * time.Second,
			want:    "select /*+ MAX_EXECUTION_TIME(2000) */ * from t",
		},
		{
			name:    "leading whitespace preserved",
			sql:     "  SELECT x",
			timeout: time.Second,
			want:    "  SELECT /*+ MAX_EXECUTION_TIME(1000) */ x",
		},
		{
			name:    "non-select unchanged",
			sql:     "INSERT INTO t VALUES (1)",
			timeout: time.Second,
			want:    "INSERT INTO t VALUES (1)",
		},
		{
			name:    "select as identifier prefix unchanged",
			sql:     "SELECTION x",
			timeout: time.Second,
			want:    "SELECTION x",
		},
		{
			name:    "WITH-prefixed select relies on client backstop",
			sql:     "WITH cte AS (SELECT 1) SELECT * FROM cte",
			timeout: time.Second,
			want:    "WITH cte AS (SELECT 1) SELECT * FROM cte",
		},
		{
			name:    "EXPLAIN-prefixed select relies on client backstop",
			sql:     "EXPLAIN SELECT 1",
			timeout: time.Second,
			want:    "EXPLAIN SELECT 1",
		},
		{
			name:    "comment-prefixed select relies on client backstop",
			sql:     "/* probe */ SELECT 1",
			timeout: time.Second,
			want:    "/* probe */ SELECT 1",
		},
		{
			name:    "disabled timeout unchanged",
			sql:     "SELECT 1",
			timeout: 0,
			want:    "SELECT 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := dynamic.StatementTimeoutHint(tt.sql, tt.timeout); got != tt.want {
				t.Fatalf("StatementTimeoutHint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatementDeadline(t *testing.T) {
	t.Parallel()

	dynamic := mysqlDialect{}

	if got := dynamic.StatementDeadline(150 * time.Millisecond); got != 150*time.Millisecond+statementTimeoutGrace {
		t.Fatalf("StatementDeadline(150ms) = %v, want 150ms + grace", got)
	}

	if got := dynamic.StatementDeadline(0); got != 0 {
		t.Fatalf("StatementDeadline(0) = %v, want 0 (disabled)", got)
	}

	if got := dynamic.StatementDeadline(-time.Second); got != -time.Second {
		t.Fatalf("StatementDeadline(-1s) = %v, want -1s (disabled)", got)
	}
}
