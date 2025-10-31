package sqlexec

import (
	"context"

	trmpgx "github.com/avito-tech/go-transaction-manager/pgxv5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlerr"
)

type ctxGetter interface {
	// DefaultTrOrDB returns the default transaction or the provided transaction
	// from the context, if it exists.
	//
	// Parameters:
	// - ctx: The context.Context object.
	// - db: The transaction to use if it exists in the context.
	//
	// Returns:
	// - trmpgx.Tr: The transaction to use.
	DefaultTrOrDB(ctx context.Context, db trmpgx.Tr) trmpgx.Tr
}

var _ Executor = (*TxExecutor)(nil)

type TxExecutor struct {
	defaultTr trmpgx.Tr
	ctxGetter ctxGetter
}

type TxExecutorOption func(*TxExecutor)

// WithCtxGetter sets the ctxGetter for the TxExecutor.
//
// Parameters:
// - ctxGetter: The ctxGetter implementation to be used.
//
// Returns:
// - TxExecutorOption: A function that takes a TxExecutor pointer and sets its ctxGetter field.
func WithCtxGetter(ctxGetter ctxGetter) TxExecutorOption {
	return func(e *TxExecutor) {
		e.ctxGetter = ctxGetter
	}
}

// NewTxExecutor creates a new TxExecutor with the given defaultTr and options.
//
// Parameters:
// - defaultTr: The default transaction to use.
// - options: Optional configurations for the TxExecutor.
//
// Returns:
// - *TxExecutor: The newly created TxExecutor.
func NewTxExecutor(defaultTr trmpgx.Tr, options ...TxExecutorOption) *TxExecutor {
	executor := &TxExecutor{
		defaultTr: defaultTr,
		ctxGetter: trmpgx.DefaultCtxGetter,
	}

	for _, option := range options {
		option(executor)
	}

	return executor
}

// tr returns the transaction to use based on the provided context.
//
// It first calls the ctxGetter's DefaultTrOrDB method to get the transaction from the context,
// or the default transaction if it doesn't exist. If the returned transaction is nil,
// it returns the default transaction. Otherwise, it returns the obtained transaction.
//
// Parameters:
// - ctx: The context.Context object.
//
// Returns:
// - trmpgx.Tr: The transaction to use.
func (e *TxExecutor) tr(ctx context.Context) trmpgx.Tr { //nolint:ireturn // lib
	tr := e.ctxGetter.DefaultTrOrDB(ctx, e.defaultTr)
	if tr == nil {
		return e.defaultTr
	}

	return tr
}

// Exec executes the given SQL statement with the provided arguments in the context of the TxExecutor.
//
// Parameters:
// - ctx: The context.Context object.
// - sql: The SQL statement to execute.
// - arguments: The arguments to be passed to the SQL statement.
//
// Returns:
// - pgconn.CommandTag: The command tag returned by the execution.
// - error: An error if the execution fails.
func (e *TxExecutor) Exec(
	ctx context.Context,
	sql string,
	arguments ...interface{},
) (pgconn.CommandTag, error) {
	tag, err := e.tr(ctx).Exec(ctx, sql, arguments...)
	if err != nil {
		return pgconn.CommandTag{}, sqlerr.SqlExecErr(err)
	}

	return tag, nil
}

// Query executes a SQL query with the provided arguments in the context of the TxExecutor.
//
// Parameters:
// - ctx: The context.Context object.
// - sql: The SQL query to execute.
// - arguments: The arguments to be passed to the SQL query.
//
// Returns:
// - pgx.Rows: The result of the query.
// - error: An error if the query execution fails.
func (e *TxExecutor) Query( //nolint:ireturn // lib
	ctx context.Context,
	sql string,
	arguments ...interface{},
) (pgx.Rows, error) {
	rows, err := e.tr(ctx).Query(ctx, sql, arguments...)
	if err != nil {
		return nil, sqlerr.SqlExecErr(err)
	}

	return rows, nil
}

// QueryRow executes a SQL query with the provided arguments in the context of the TxExecutor,
// returning a single row result.
//
// Parameters:
// - ctx: The context.Context object.
// - sql: The SQL query to execute.
// - arguments: The arguments to be passed to the SQL query.
//
// Returns:
// - pgx.Row: The result of the query as a single row.
func (e *TxExecutor) QueryRow( //nolint:ireturn // lib
	ctx context.Context,
	sql string,
	arguments ...interface{},
) pgx.Row {
	return e.tr(ctx).QueryRow(ctx, sql, arguments...)
}

func (e *TxExecutor) CopyFrom(
	ctx context.Context,
	tableName pgx.Identifier,
	columnNames []string,
	rowSrc pgx.CopyFromSource,
) (int64, error) {
	return e.tr(ctx).CopyFrom(ctx, tableName, columnNames, rowSrc)
}

func (e *TxExecutor) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return e.tr(ctx).SendBatch(ctx, b)
}
