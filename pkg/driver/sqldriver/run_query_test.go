package sqldriver

import (
	"errors"
	"fmt"
	"testing"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
)

var _ queries.Dialect = testDialect{}

type testDialect struct{}

func (testDialect) Placeholder(_ int) string { return "?" }
func (testDialect) ValueToAny(_ *stroppy.Value) (any, error) {
	return nil, nil //nolint:nilnil // test stub
}
func (testDialect) Deduplicate() bool { return false }

func TestProcessArgs(t *testing.T) {
	dialect := testDialect{}

	tests := []struct {
		name     string
		sql      string
		args     map[string]any
		wantSQL  string
		wantArgs []any
		wantErr  error
	}{
		{
			name:     "single argument",
			sql:      "SELECT * FROM users WHERE id = :user_id;",
			args:     map[string]any{"user_id": 123},
			wantSQL:  "SELECT * FROM users WHERE id = ?;",
			wantArgs: []any{123},
		},
		{
			name: "multiple arguments",
			sql: `SELECT * FROM users
			       WHERE name = :user_name AND age > :min_age
			       AND status = :status`,
			args: map[string]any{
				"user_name": "John",
				"min_age":   18,
				"status":    true,
			},
			wantSQL: `SELECT * FROM users
			       WHERE name = ? AND age > ?
			       AND status = ?`,
			wantArgs: []any{"John", 18, true},
		},
		{
			name:     "duplicate arg emits value twice",
			sql:      "SELECT * FROM users WHERE id = :user_id AND backup_id = :user_id ",
			args:     map[string]any{"user_id": 123},
			wantSQL:  "SELECT * FROM users WHERE id = ? AND backup_id = ? ",
			wantArgs: []any{123, 123},
		},
		{
			name:     "no arguments in query",
			sql:      "SELECT * FROM users WHERE active = true",
			args:     map[string]any{},
			wantSQL:  "SELECT * FROM users WHERE active = true",
			wantArgs: nil,
		},
		{
			name:     "comma separated values",
			sql:      `INSERT INTO t (a, b) VALUES (:x, :y)`,
			args:     map[string]any{"x": 1, "y": "hello"},
			wantSQL:  `INSERT INTO t (a, b) VALUES (?, ?)`,
			wantArgs: []any{1, "hello"},
		},
		{
			name:     "missing argument",
			sql:      "SELECT * FROM users WHERE id = :user_id ",
			args:     map[string]any{},
			wantSQL:  "",
			wantArgs: nil,
			wantErr:  ErrMissedArgument,
		},
		{
			name:     "multiple missing arguments",
			sql:      "SELECT * FROM t WHERE a = :x AND b = :y AND c = :z",
			args:     map[string]any{"x": 1},
			wantSQL:  "",
			wantArgs: nil,
			wantErr:  ErrMissedArgument,
		},
		{
			name:     "extra argument",
			sql:      "SELECT 1",
			args:     map[string]any{"unused": 42},
			wantSQL:  "SELECT 1",
			wantArgs: nil,
			wantErr:  ErrExtraArgument,
		},
		{
			name:     "typecast syntax",
			sql:      "SELECT * FROM users WHERE id = :user_id::int;",
			args:     map[string]any{"user_id": 123},
			wantSQL:  "SELECT * FROM users WHERE id = ?::int;",
			wantArgs: []any{123},
		},
	}

	t.Parallel()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotSQL, gotArgs, gotErr := ProcessArgs(dialect, tt.sql, tt.args)

			if tt.wantErr != nil {
				if gotErr == nil {
					t.Fatalf("ProcessArgs() error = nil, want %v", tt.wantErr)
				}

				if !errors.Is(gotErr, tt.wantErr) {
					t.Fatalf("ProcessArgs() error = %v, want %v", gotErr, tt.wantErr)
				}

				return
			}

			if gotErr != nil {
				t.Fatalf("ProcessArgs() unexpected error: %v", gotErr)
			}

			if gotSQL != tt.wantSQL {
				t.Errorf("ProcessArgs() SQL = %q, want %q", gotSQL, tt.wantSQL)
			}

			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("ProcessArgs() args len = %d, want %d", len(gotArgs), len(tt.wantArgs))
			}

			for i, v := range gotArgs {
				if v != tt.wantArgs[i] {
					t.Errorf("ProcessArgs() args[%d] = %v, want %v", i, v, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestProcessArgsDuplicateArgNoDedupExtraValues(t *testing.T) {
	t.Parallel()

	dialect := testDialect{}
	sql := "SELECT * FROM t WHERE a = :id AND b = :id AND c = :id "
	args := map[string]any{"id": 42}

	gotSQL, gotArgs, err := ProcessArgs(dialect, sql, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "SELECT * FROM t WHERE a = ? AND b = ? AND c = ? "
	if gotSQL != wantSQL {
		t.Errorf("SQL = %q, want %q", gotSQL, wantSQL)
	}

	if len(gotArgs) != 3 {
		t.Fatalf("args len = %d, want 3", len(gotArgs))
	}

	for i, v := range gotArgs {
		if v != 42 {
			t.Errorf("args[%d] = %v, want 42", i, v)
		}
	}
}

func TestProcessArgsMissedErrorMessage(t *testing.T) {
	t.Parallel()

	dialect := testDialect{}

	_, _, err := ProcessArgs(dialect, "SELECT :a AND :b ", map[string]any{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrMissedArgument) {
		t.Fatalf("expected ErrMissedArgument, got %v", err)
	}

	want := fmt.Sprintf("%s: [a, b]", ErrMissedArgument)
	if err.Error() != want {
		t.Errorf("error message = %q, want %q", err.Error(), want)
	}
}
