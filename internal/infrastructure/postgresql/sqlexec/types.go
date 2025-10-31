package sqlexec

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Executor interface {
	// Exec executes the given SQL statement with the provided arguments in the context of the Executor.
	//
	// Parameters:
	// - ctx: The context.Context object.
	// - sql: The SQL statement to execute.
	// - arguments: The arguments to be passed to the SQL statement.
	//
	// Returns:
	// - pgconn.CommandTag: The command tag returned by the execution.
	// - error: An error if the execution fails.
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	// Query executes a SQL query with the provided arguments in the context of the Executor.
	//
	// Parameters:
	// - ctx: The context.Context object.
	// - sql: The SQL query to execute.
	// - arguments: The arguments to be passed to the SQL query.
	//
	// Returns:
	// - pgx.Rows: The result of the query.
	// - error: An error if the query execution fails.
	Query(ctx context.Context, sql string, arguments ...interface{}) (pgx.Rows, error)
	// QueryRow executes a SQL query with the provided arguments in the context of the Executor,
	// returning a single row result.
	//
	// Parameters:
	// - ctx: The context.Context object.
	// - sql: The SQL query to execute.
	// - arguments: The arguments to be passed to the SQL query.
	//
	// Returns:
	// - pgx.Row: The result of the query as a single row.
	QueryRow(ctx context.Context, sql string, arguments ...interface{}) pgx.Row

	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)

	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

var _ Executor = (*TxExecutor)(nil)

type SqlizerExecutorInterface interface {
	// Exec executes the SQL statement represented by the given Sqlizer and returns
	// the command tag and any error encountered.
	//
	// ctx: The context.Context to use for the execution.
	// builder: The Sqlizer representing the SQL statement to execute.
	// Returns the pgconn.CommandTag representing the result of the execution and
	// any error encountered.
	Exec(ctx context.Context, builder Sqlizer) (pgconn.CommandTag, error)
	// Query executes a SQL query using the provided context and SQL builder.
	//
	// Parameters:
	//   - ctx: The context.Context object used for cancellation and timeout.
	//   - builder: The Sqlizer object used to build the SQL query.
	//
	// Returns:
	//   - pgx.Rows: The result set of the executed query.
	//   - error: An error object if the query execution fails.
	Query(ctx context.Context, builder Sqlizer) (pgx.Rows, error)
	// QueryRow executes a SQL query with the provided arguments in the context of the Executor,
	// returning a single row result.
	//
	// Parameters:
	// - ctx: The context.Context object.
	// - builder: The Sqlizer representing the SQL query to execute.
	//
	// Returns:
	// - pgx.Row: The result of the query as a single row.
	QueryRow(ctx context.Context, builder Sqlizer) pgx.Row
}

var _ SqlizerExecutorInterface = (*SqlizerExecutor)(nil)
