package postgres

import (
	"errors"
	"fmt"
	"testing"
)

func TestProcessArgs(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		args     map[string]any
		wantSQL  string
		wantArgs []any
		wantErr  error
	}{
		{
			name:     "semicolon syntax",
			sql:      "SELECT * FROM users WHERE id = :user_id;",
			args:     map[string]any{"user_id": 123},
			wantSQL:  "SELECT * FROM users WHERE id = $1;",
			wantArgs: []any{123},
			wantErr:  nil,
		},
		{
			name:     "typecast syntax",
			sql:      "SELECT * FROM users WHERE id = :user_id::int;",
			args:     map[string]any{"user_id": 123},
			wantSQL:  "SELECT * FROM users WHERE id = $1::int;",
			wantArgs: []any{123},
			wantErr:  nil,
		},
		{
			name:     "single argument success",
			sql:      "SELECT * FROM users WHERE id = :user_id ",
			args:     map[string]any{"user_id": 123},
			wantSQL:  "SELECT * FROM users WHERE id = $1 ",
			wantArgs: []any{123},
			wantErr:  nil,
		},
		{
			name: "multiple arguments success",
			sql: `SELECT * FROM users
				       WHERE name = :user_name AND age > :min_age
				       AND status = :status`,
			args: map[string]any{
				"user_name": "John",
				"min_age":   18,
				"status":    true,
			},
			wantSQL: `SELECT * FROM users
				       WHERE name = $1 AND age > $2
				       AND status = $3`,
			wantArgs: []any{"John", 18, true},
			wantErr:  nil,
		},
		{
			name: "multiline with tabs and spaces",
			sql: `SELECT u.id, u.name
		FROM users u
		JOIN orders o ON u.id = o.user_id
		WHERE u.created_at >= :start_date
		  AND u.created_at <= :end_date
		  AND o.total > :min_total`,
			args: map[string]any{
				"start_date": "2023-01-01",
				"end_date":   "2023-12-31",
				"min_total":  100.0,
			},
			wantSQL: `SELECT u.id, u.name
		FROM users u
		JOIN orders o ON u.id = o.user_id
		WHERE u.created_at >= $1
		  AND u.created_at <= $2
		  AND o.total > $3`,
			wantArgs: []any{"2023-01-01", "2023-12-31", 100.0},
			wantErr:  nil,
		},
		{
			name:     "missing single argument",
			sql:      "SELECT * FROM users WHERE id = :user_id ",
			args:     map[string]any{},
			wantSQL:  "",
			wantArgs: nil,
			wantErr:  fmt.Errorf("%w: [user_id]", ErrMissedArgument),
		},
		{
			name:     "multiple missing arguments",
			sql:      "SELECT * FROM users WHERE id = :user_id AND name = :user_name AND age = :age",
			args:     map[string]any{"user_id": 123},
			wantSQL:  "",
			wantArgs: nil,
			wantErr:  fmt.Errorf("%w: [user_name, age]", ErrMissedArgument),
		},
		{
			name:     "duplicate arguments",
			sql:      "SELECT * FROM users WHERE id = :user_id AND backup_id = :user_id ",
			args:     map[string]any{"user_id": 123},
			wantSQL:  "SELECT * FROM users WHERE id = $1 AND backup_id = $1 ",
			wantArgs: []any{123},
			wantErr:  nil,
		},
		{
			name:     "no arguments",
			sql:      "SELECT * FROM users WHERE active = true",
			args:     map[string]any{},
			wantSQL:  "SELECT * FROM users WHERE active = true",
			wantArgs: nil,
			wantErr:  nil,
		},
		{
			name:     "arguments with numbers and underscores",
			sql:      "WHERE user_123 = :param_456 AND version_2 = :v2 ",
			args:     map[string]any{"param_456": "value", "v2": 2},
			wantSQL:  "WHERE user_123 = $1 AND version_2 = $2 ",
			wantArgs: []any{"value", 2},
			wantErr:  nil,
		},
		{
			name:     "no match due to missing spaces",
			sql:      `SELECT * FROM users WHERE id=:user_id AND name=:user_name`,
			args:     map[string]any{"user_id": 123, "user_name": "John"},
			wantSQL:  `SELECT * FROM users WHERE id=:user_id AND name=:user_name`,
			wantArgs: nil,
			wantErr:  fmt.Errorf("%w: [user_id, user_name]", ErrExtraArgument),
		},
		{
			name:     "comma test",
			sql:      `INSERT INTO pgbench_branches (bid, bbalance, filler) VALUES (:branch, 0, :filler)`,
			args:     map[string]any{"branch": 123, "filler": "John"},
			wantSQL:  `INSERT INTO pgbench_branches (bid, bbalance, filler) VALUES ($1, 0, $2)`,
			wantArgs: []any{123, "John"},
			wantErr:  nil,
		},
	}

	t.Parallel()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotSQL, gotArgs, gotErr := processArgs(tt.sql, tt.args)

			// Check SQL output
			if gotSQL != tt.wantSQL {
				t.Errorf("processArgs() SQL = %q, want %q", gotSQL, tt.wantSQL)
			}

			// Check args slice
			if len(gotArgs) != len(tt.wantArgs) {
				t.Errorf("processArgs() args len = %d, want %d", len(gotArgs), len(tt.wantArgs))
			} else {
				for i, v := range gotArgs {
					if v != tt.wantArgs[i] {
						t.Errorf("processArgs() args[%d] = %v, want %v", i, v, tt.wantArgs[i])
					}
				}
			}

			// Check error
			if tt.wantErr != nil {
				if gotErr == nil {
					t.Errorf("processArgs() error = nil, want %v", tt.wantErr)

					return
				}

				if !errors.Is(gotErr, ErrMissedArgument) && !errors.Is(gotErr, ErrExtraArgument) {
					t.Errorf(
						"processArgs() error type mismatch, got %v, want ErrMissedArgument or ErrExtraArgument",
						gotErr,
					)

					return
				}
				// Additional check for error message content
				if gotErr.Error() != tt.wantErr.Error() {
					t.Errorf(
						"processArgs() error message = %q, want %q",
						gotErr.Error(),
						tt.wantErr.Error(),
					)
				}
			} else if gotErr != nil {
				t.Errorf("processArgs() error = %v, want nil", gotErr)
			}
		})
	}
}
