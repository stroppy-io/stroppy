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
