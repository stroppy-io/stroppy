package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
)

func ptr[T any](v T) *T {
	return &v
}

var _ = ptr[int64]

type testDriver struct {
	*Driver
}

func newTestDriver(mockPool pgxmock.PgxPoolIface) (*testDriver, error) {
	builder, err := queries.NewQueryBuilder(0)
	if err != nil {
		return nil, err
	}

	return &testDriver{
		Driver: &Driver{
			logger:  logger.Global(),
			pgxPool: mockPool,
			builder: builder,
		},
	}, nil
}

func TestDriver_runTransaction(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer mock.Close()

	drv, err := newTestDriver(mock)
	require.NoError(t, err)

	ctx := context.Background()
	query := &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{
			{
				Name:    "test_query",
				Request: "SELECT 1",
				Params:  nil,
			},
		},
	}

	mock.ExpectExec("SELECT 1").WillReturnResult(pgxmock.NewResult("SELECT", 1))

	stats, err := drv.runTransaction(ctx, query)
	require.NoError(t, err)
	require.NotEmpty(t, stats)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDriver_InsertValuesPlainQuery(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer mock.Close()

	drv, err := newTestDriver(mock)
	require.NoError(t, err)

	ctx := context.Background()
	descriptor := &stroppy.InsertDescriptor{
		Name:      "test_insert",
		TableName: "test_table",
		Method:    stroppy.InsertMethod_PLAIN_QUERY.Enum(),
		Params: []*stroppy.QueryParamDescriptor{
			{
				Name: "id",
				GenerationRule: &stroppy.Generation_Rule{
					Kind: &stroppy.Generation_Rule_Int64Range{
						Int64Range: &stroppy.Generation_Range_Int64{
							Min: ptr[int64](1),
							Max: 100,
						},
					},
					Unique: ptr(true),
				},
			},
		},
	}

	count := int64(3)

	// Expect 3 insert executions
	for range count {
		mock.ExpectExec("insert into test_table").
			WithArgs(pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
	}

	stats, err := drv.InsertValues(ctx, descriptor, count)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Queries, 1)
	require.Equal(t, "test_insert", stats.Queries[0].Name)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDriver_InsertValuesCopyFrom(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer mock.Close()

	drv, err := newTestDriver(mock)
	require.NoError(t, err)

	ctx := context.Background()
	descriptor := &stroppy.InsertDescriptor{
		Name:      "test_insert_copy",
		TableName: "test_table",
		Method:    stroppy.InsertMethod_COPY_FROM.Enum(),
		Params: []*stroppy.QueryParamDescriptor{
			{
				Name: "id",
				GenerationRule: &stroppy.Generation_Rule{
					Kind: &stroppy.Generation_Rule_Int64Range{
						Int64Range: &stroppy.Generation_Range_Int64{
							Min: ptr[int64](1),
							Max: 100,
						},
					},
					Unique: ptr(true),
				},
			},
			{
				Name: "name",
				GenerationRule: &stroppy.Generation_Rule{
					Kind: &stroppy.Generation_Rule_StringConst{
						StringConst: "test_name",
					},
				},
			},
		},
	}

	count := int64(5)

	// Expect one CopyFrom call with 5 rows
	mock.ExpectCopyFrom(
		[]string{"test_table"},
		[]string{"id", "name"},
	).WillReturnResult(count)

	stats, err := drv.InsertValues(ctx, descriptor, count)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Queries, 1)
	require.Equal(t, "test_insert_copy", stats.Queries[0].Name)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDriver_InsertValuesCopyFromLargeBatch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer mock.Close()

	drv, err := newTestDriver(mock)
	require.NoError(t, err)

	ctx := context.Background()
	descriptor := &stroppy.InsertDescriptor{
		Name:      "test_insert_large",
		TableName: "test_table",
		Method:    stroppy.InsertMethod_COPY_FROM.Enum(),
		Params: []*stroppy.QueryParamDescriptor{
			{
				Name: "id",
				GenerationRule: &stroppy.Generation_Rule{
					Kind: &stroppy.Generation_Rule_Int64Range{
						Int64Range: &stroppy.Generation_Range_Int64{
							Min: ptr[int64](1),
							Max: 1000000,
						},
					},
					Unique: ptr(true),
				},
			},
			{
				Name: "value",
				GenerationRule: &stroppy.Generation_Rule{
					Kind: &stroppy.Generation_Rule_Int64Range{
						Int64Range: &stroppy.Generation_Range_Int64{
							Min: ptr[int64](1),
							Max: 1000,
						},
					},
				},
			},
		},
	}

	count := int64(10000)

	// Expect one CopyFrom call with 10000 rows - demonstrates streaming without memory issues
	mock.ExpectCopyFrom(
		[]string{"test_table"},
		[]string{"id", "value"},
	).WillReturnResult(count)

	stats, err := drv.InsertValues(ctx, descriptor, count)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Queries, 1)
	require.Equal(t, "test_insert_large", stats.Queries[0].Name)

	require.NoError(t, mock.ExpectationsWereMet())
}

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
			wantErr:  fmt.Errorf("%w: user_id", ErrMissedArgument),
		},
		{
			name:     "multiple missing arguments",
			sql:      "SELECT * FROM users WHERE id = :user_id AND name = :user_name AND age = :age",
			args:     map[string]any{"user_id": 123},
			wantSQL:  "",
			wantArgs: nil,
			wantErr:  fmt.Errorf("%w: user_name, age", ErrMissedArgument),
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
			wantErr:  ErrExtraArgument,
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
