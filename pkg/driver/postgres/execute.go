package postgres

import (
	"context"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
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
}

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

// NewTxExecutor creates a new TxExecutor with the given defaultTr and options.
//
// Parameters:
// - defaultTr: The default transaction to use.
//
// Returns:
// - *TxExecutor: The newly created TxExecutor.
func NewTxExecutor(defaultTr trmpgx.Tr) *TxExecutor {
	executor := &TxExecutor{
		defaultTr: defaultTr,
		ctxGetter: trmpgx.DefaultCtxGetter,
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
		return pgconn.CommandTag{}, err
	}

	return tag, nil
}
