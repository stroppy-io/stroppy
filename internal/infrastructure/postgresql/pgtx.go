package postgres

import (
	"context"
	"fmt"

	"github.com/avito-tech/go-transaction-manager/pgxv5"
	trmpgx "github.com/avito-tech/go-transaction-manager/pgxv5"
	"github.com/avito-tech/go-transaction-manager/trm"
	"github.com/avito-tech/go-transaction-manager/trm/manager"
	"github.com/avito-tech/go-transaction-manager/trm/settings"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlexec"
)

// NewTxFlow creates a new transaction flow with the given pgx pool and options.
//
// It takes a pgx pool and a variadic number of TxExecutorOption as arguments.
// It returns a pointer to a TxExecutor, a pointer to a manager.Manager, and an error.
func NewTxFlow(psql *pgxpool.Pool, settings *trmpgx.Settings, options ...sqlexec.TxExecutorOption) (*sqlexec.TxExecutor, *manager.Manager, error) {
	txManager, err := NewTXManager(psql, settings)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres.NewTXManager: %w", err)
	}

	return sqlexec.NewTxExecutor(psql, options...), txManager, nil
}

func NewSettings(level pgx.TxIsoLevel, opts ...settings.Opt) *trmpgx.Settings {
	setts := pgxv5.MustSettings(settings.Must(opts...),
		pgxv5.WithTxOptions(pgx.TxOptions{
			IsoLevel: level,
		}),
	)
	return &setts
}

func SerializableSettings(opts ...settings.Opt) *trmpgx.Settings {
	return NewSettings(pgx.Serializable, opts...)
}

func RepeatableReadSettings(opts ...settings.Opt) *trmpgx.Settings {
	return NewSettings(pgx.RepeatableRead, opts...)
}

func ReadCommittedSettings(opts ...settings.Opt) *trmpgx.Settings {
	return NewSettings(pgx.ReadCommitted, opts...)
}

func ReadUncommittedSettings(opts ...settings.Opt) *trmpgx.Settings {
	return NewSettings(pgx.ReadUncommitted, opts...)
}

type TxManager = trm.Manager

func parseError(err error) error {
	if err == nil {
		return nil
	}
	return err
}

func WithTransactionRet[T any](
	ctx context.Context,
	mgr TxManager,
	level pgx.TxIsoLevel,
	f func(ctx context.Context) (T, error),
	opts ...settings.Opt,
) (ret T, err error) {
	err = mgr.DoWithSettings(ctx,
		NewSettings(level, opts...),
		func(ctx context.Context) error {
			ret, err = f(ctx)
			return err
		},
	)
	return ret, err
}

func WithSerializableRet[T any](
	ctx context.Context,
	manager TxManager,
	f func(ctx context.Context) (T, error),
	opts ...settings.Opt,
) (ret T, err error) {
	return WithTransactionRet(ctx, manager, pgx.Serializable, f, opts...)
}
func WithReadCommittedRet[T any](
	ctx context.Context,
	manager TxManager,
	f func(ctx context.Context) (T, error),
	opts ...settings.Opt,
) (ret T, err error) {
	return WithTransactionRet(ctx, manager, pgx.ReadCommitted, f, opts...)
}
func WithReadUncommittedRet[T any](
	ctx context.Context,
	manager TxManager,
	f func(ctx context.Context) (T, error),
	opts ...settings.Opt,
) (ret T, err error) {
	return WithTransactionRet(ctx, manager, pgx.ReadUncommitted, f, opts...)
}
func WithRepeatableReadRet[T any](
	ctx context.Context,
	manager TxManager,
	f func(ctx context.Context) (T, error),
	opts ...settings.Opt,
) (ret T, err error) {
	return WithTransactionRet(ctx, manager, pgx.RepeatableRead, f, opts...)
}

func WithTransaction(
	ctx context.Context,
	txManager TxManager,
	level pgx.TxIsoLevel,
	fn func(ctx context.Context) error,
	opts ...settings.Opt,
) error {
	return txManager.DoWithSettings(ctx, NewSettings(level, opts...), fn)
}

func WithSerializable(
	ctx context.Context,
	manager TxManager,
	fn func(ctx context.Context) error,
	opts ...settings.Opt,
) error {
	return WithTransaction(ctx, manager, pgx.Serializable, fn, opts...)
}
func WithRepeatableRead(
	ctx context.Context,
	manager TxManager,
	fn func(ctx context.Context) error,
	opts ...settings.Opt,
) error {
	return WithTransaction(ctx, manager, pgx.RepeatableRead, fn, opts...)
}
func WithReadCommitted(
	ctx context.Context,
	manager TxManager,
	fn func(ctx context.Context) error,
	opts ...settings.Opt,
) error {
	return WithTransaction(ctx, manager, pgx.ReadCommitted, fn, opts...)
}
func WithReadUncommitted(
	ctx context.Context,
	manager TxManager,
	fn func(ctx context.Context) error,
	opts ...settings.Opt,
) error {
	return WithTransaction(ctx, manager, pgx.ReadUncommitted, fn, opts...)
}
