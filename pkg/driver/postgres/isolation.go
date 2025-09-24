package postgres

import (
	"errors"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/settings"
	"github.com/jackc/pgx/v5"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

func NewSettings(level pgx.TxIsoLevel, opts ...settings.Opt) *trmpgx.Settings {
	setts := trmpgx.MustSettings(settings.Must(opts...),
		trmpgx.WithTxOptions(pgx.TxOptions{
			IsoLevel: level,
		}),
	)

	return &setts
}

func ReadUncommittedSettings(opts ...settings.Opt) *trmpgx.Settings {
	return NewSettings(pgx.ReadUncommitted, opts...)
}

func ReadCommittedSettings(opts ...settings.Opt) *trmpgx.Settings {
	return NewSettings(pgx.ReadCommitted, opts...)
}

func RepeatableReadSettings(opts ...settings.Opt) *trmpgx.Settings {
	return NewSettings(pgx.RepeatableRead, opts...)
}

func SerializableSettings(opts ...settings.Opt) *trmpgx.Settings {
	return NewSettings(pgx.Serializable, opts...)
}

var ErrUnsupportedIsolationLevel = errors.New("unsupported isolation level")

func NewStroppyIsolationSettings(transaction *stroppy.DriverTransaction, opts ...settings.Opt) *trmpgx.Settings {
	switch transaction.GetIsolationLevel() {
	case stroppy.TxIsolationLevel_TX_ISOLATION_LEVEL_READ_UNCOMMITTED:
		return ReadUncommittedSettings(opts...)
	case stroppy.TxIsolationLevel_TX_ISOLATION_LEVEL_READ_COMMITTED:
		return ReadCommittedSettings(opts...)
	case stroppy.TxIsolationLevel_TX_ISOLATION_LEVEL_REPEATABLE_READ:
		return RepeatableReadSettings(opts...)
	case stroppy.TxIsolationLevel_TX_ISOLATION_LEVEL_SERIALIZABLE:
		return SerializableSettings(opts...)
	default:
		panic(ErrUnsupportedIsolationLevel)
	}
}
