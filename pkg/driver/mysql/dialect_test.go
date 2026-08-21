package mysql

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestStatementTimeoutHint(t *testing.T) {
	t.Parallel()

	dialect := mysqlDialect{}
	maxTimeout := time.Duration(math.MaxUint32) * time.Millisecond

	tests := []struct {
		name       string
		sql        string
		timeout    time.Duration
		want       string
		wantActive bool
	}{
		{
			name:       "select gets hint after keyword",
			sql:        "SELECT 1",
			timeout:    time.Second,
			want:       "SELECT /*+ MAX_EXECUTION_TIME(1000) */ 1",
			wantActive: true,
		},
		{
			name:       "lowercase select preserves its keyword case",
			sql:        "select * from t",
			timeout:    2 * time.Second,
			want:       "select /*+ MAX_EXECUTION_TIME(2000) */ * from t",
			wantActive: true,
		},
		{
			name:       "leading whitespace preserved",
			sql:        "  SELECT x",
			timeout:    time.Second,
			want:       "  SELECT /*+ MAX_EXECUTION_TIME(1000) */ x",
			wantActive: true,
		},
		{
			name:       "existing optimizer hints share one block",
			sql:        "SeLeCt\t/*+ SET_VAR(sort_buffer_size=32768) BKA(t) */\n1",
			timeout:    1500 * time.Millisecond,
			want:       "SeLeCt\t/*+ SET_VAR(sort_buffer_size=32768) BKA(t) MAX_EXECUTION_TIME(1500) */\n1",
			wantActive: true,
		},
		{
			name:       "adjacent optimizer hint block is merged",
			sql:        "SELECT/*+SET_VAR(sort_buffer_size=32768)*/1",
			timeout:    time.Second,
			want:       "SELECT/*+SET_VAR(sort_buffer_size=32768) MAX_EXECUTION_TIME(1000)*/1",
			wantActive: true,
		},
		{
			name:       "existing timeout spacing and case are replaced",
			sql:        "SELECT /*+ set_var(sort_buffer_size=32768) mAx_ExEcUtIoN_tImE \n ( \t5000 ) BKA(t) */ 1",
			timeout:    250 * time.Millisecond,
			want:       "SELECT /*+ set_var(sort_buffer_size=32768) MAX_EXECUTION_TIME(250) BKA(t) */ 1",
			wantActive: true,
		},
		{
			name:       "all duplicate timeouts collapse to configured timeout",
			sql:        "SELECT /*+ MAX_EXECUTION_TIME(1) SET_VAR(sort_buffer_size=32768) max_execution_time (2) */ 1",
			timeout:    250 * time.Millisecond,
			want:       "SELECT /*+ MAX_EXECUTION_TIME(250) SET_VAR(sort_buffer_size=32768)  */ 1",
			wantActive: true,
		},
		{
			name:       "canonical timeout remains active without a rewrite",
			sql:        "SELECT /*+ MAX_EXECUTION_TIME(1000) */ 1",
			timeout:    time.Second,
			want:       "SELECT /*+ MAX_EXECUTION_TIME(1000) */ 1",
			wantActive: true,
		},
		{
			name:    "select sleep remains on exact client deadline",
			sql:     "SELECT SLEEP(10) AS slept",
			timeout: time.Second,
			want:    "SELECT SLEEP(10) AS slept",
		},
		{
			name:    "select sleep accepts keyword spacing and case",
			sql:     "select\nSlEeP \t (10)",
			timeout: time.Second,
			want:    "select\nSlEeP \t (10)",
		},
		{
			name:    "select sleep after optimizer hints remains unchanged",
			sql:     "SELECT /*+ SET_VAR(sort_buffer_size=32768) */ SLEEP(10)",
			timeout: time.Second,
			want:    "SELECT /*+ SET_VAR(sort_buffer_size=32768) */ SLEEP(10)",
		},
		{
			name:    "select sleep after ordinary block comment remains unchanged",
			sql:     "SELECT /* probe */ SLEEP(10)",
			timeout: time.Second,
			want:    "SELECT /* probe */ SLEEP(10)",
		},
		{
			name:       "sleep identifier prefix remains eligible",
			sql:        "SELECT SLEEPING(10)",
			timeout:    time.Second,
			want:       "SELECT /*+ MAX_EXECUTION_TIME(1000) */ SLEEPING(10)",
			wantActive: true,
		},
		{
			name:       "select expression without whitespace is eligible",
			sql:        "SELECT(1)",
			timeout:    time.Second,
			want:       "SELECT /*+ MAX_EXECUTION_TIME(1000) */(1)",
			wantActive: true,
		},
		{
			name:    "non-select unchanged",
			sql:     "INSERT INTO t VALUES (1)",
			timeout: time.Second,
			want:    "INSERT INTO t VALUES (1)",
		},
		{
			name:    "do unchanged",
			sql:     "DO SLEEP(10)",
			timeout: time.Second,
			want:    "DO SLEEP(10)",
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
		{
			name:    "negative timeout unchanged",
			sql:     "SELECT 1",
			timeout: -time.Second,
			want:    "SELECT 1",
		},
		{
			name:    "sub-millisecond timeout unchanged",
			sql:     "SELECT 1",
			timeout: time.Millisecond - time.Nanosecond,
			want:    "SELECT 1",
		},
		{
			name:       "one millisecond is representable",
			sql:        "SELECT 1",
			timeout:    time.Millisecond,
			want:       "SELECT /*+ MAX_EXECUTION_TIME(1) */ 1",
			wantActive: true,
		},
		{
			name:       "fractional milliseconds use representable integer part",
			sql:        "SELECT 1",
			timeout:    time.Millisecond + 500*time.Microsecond,
			want:       "SELECT /*+ MAX_EXECUTION_TIME(1) */ 1",
			wantActive: true,
		},
		{
			name:       "uint32 maximum milliseconds is representable",
			sql:        "SELECT 1",
			timeout:    maxTimeout,
			want:       "SELECT /*+ MAX_EXECUTION_TIME(4294967295) */ 1",
			wantActive: true,
		},
		{
			name:    "duration above uint32 maximum unchanged",
			sql:     "SELECT 1",
			timeout: maxTimeout + time.Nanosecond,
			want:    "SELECT 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hintedSQL, active := dialect.StatementTimeoutHint(tt.sql, tt.timeout)
			if hintedSQL != tt.want || active != tt.wantActive {
				t.Fatalf(
					"StatementTimeoutHint() = (%q, %t), want (%q, %t)",
					hintedSQL,
					active,
					tt.want,
					tt.wantActive,
				)
			}
		})
	}
}

func TestStatementTimeoutHintTopLevelSleepWrappers(t *testing.T) {
	t.Parallel()

	dialect := mysqlDialect{}
	selectPrefixes := []string{
		"SELECT /* expression */ ",
		"SELECT /*+ SET_VAR(sort_buffer_size=32768) */ /* expression */ ",
	}
	calls := []string{
		"SLEEP(10)",
		"sleep /* call */ (10)",
		"SlEeP(IF(1, 10, 0))",
	}

	for depth := range 5 {
		opening := strings.Repeat("( /* open */ ", depth)
		closing := strings.Repeat(" /* close */ )", depth)

		for _, prefix := range selectPrefixes {
			for _, call := range calls {
				sql := prefix + opening + "/* before */ " + call + " /* after */" + closing + " AS slept"
				hintedSQL, active := dialect.StatementTimeoutHint(sql, time.Second)

				if hintedSQL != sql || active {
					t.Errorf(
						"depth %d StatementTimeoutHint(%q) = (%q, %t), want (%q, false)",
						depth,
						sql,
						hintedSQL,
						active,
						sql,
					)
				}
			}
		}
	}
}

func TestStatementTimeoutHintDoesNotPromoteNestedSleep(t *testing.T) {
	t.Parallel()

	dialect := mysqlDialect{}
	expressions := []string{
		"ABS(SLEEP(10))",
		"(ABS(SLEEP(10)))",
		"SLEEP(10) + 1",
		"(SLEEP(10) + 1)",
		"(SLEEP(10)) + 1",
		"(SELECT SLEEP(10))",
	}

	for _, expression := range expressions {
		sql := "SELECT " + expression
		want := "SELECT /*+ MAX_EXECUTION_TIME(1000) */ " + expression
		hintedSQL, active := dialect.StatementTimeoutHint(sql, time.Second)

		if hintedSQL != want || !active {
			t.Errorf(
				"StatementTimeoutHint(%q) = (%q, %t), want (%q, true)",
				sql,
				hintedSQL,
				active,
				want,
			)
		}
	}
}

func TestStatementDeadline(t *testing.T) {
	t.Parallel()

	dialect := mysqlDialect{}
	maxTimeout := time.Duration(math.MaxUint32) * time.Millisecond

	tests := []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{name: "disabled", timeout: 0, want: 0},
		{name: "negative", timeout: -time.Second, want: -time.Second},
		{
			name:    "sub-millisecond remains exact",
			timeout: time.Millisecond - time.Nanosecond,
			want:    time.Millisecond - time.Nanosecond,
		},
		{
			name:    "minimum hint gets grace",
			timeout: time.Millisecond,
			want:    time.Millisecond + statementTimeoutGrace,
		},
		{
			name:    "fractional milliseconds get grace",
			timeout: time.Millisecond + 500*time.Microsecond,
			want:    time.Millisecond + 500*time.Microsecond + statementTimeoutGrace,
		},
		{
			name:    "maximum hint gets grace",
			timeout: maxTimeout,
			want:    maxTimeout + statementTimeoutGrace,
		},
		{
			name:    "above maximum remains exact",
			timeout: maxTimeout + time.Nanosecond,
			want:    maxTimeout + time.Nanosecond,
		},
		{
			name:    "maximum duration remains exact",
			timeout: time.Duration(math.MaxInt64),
			want:    time.Duration(math.MaxInt64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := dialect.StatementDeadline(tt.timeout); got != tt.want {
				t.Fatalf("StatementDeadline(%v) = %v, want %v", tt.timeout, got, tt.want)
			}
		})
	}
}
