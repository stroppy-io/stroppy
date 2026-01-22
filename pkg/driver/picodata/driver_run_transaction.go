package picodata

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// RunTransaction executes all queries in the transaction one by one,
// since Picodata doesn't support transactions.
func (d *Driver) RunTransaction(
	ctx context.Context,
	unit *stroppy.UnitDescriptor,
) (*stroppy.DriverTransactionStat, error) {
	err := d.builder.AddGenerators(unit)
	if err != nil {
		return nil, err
	}

	transaction, err := d.GenerateNextUnit(ctx, unit)
	if err != nil {
		return nil, err
	}

	return d.runQueriesSequentially(ctx, transaction)
}

func (d *Driver) runQueriesSequentially(
	ctx context.Context,
	transaction *stroppy.DriverTransaction,
) (*stroppy.DriverTransactionStat, error) {
	if transaction.GetIsolationLevel() != stroppy.TxIsolationLevel_UNSPECIFIED {
		d.logger.Warn(
			"Picodata doesn't support transactions, ignoring isolation level",
			zap.String("requested_isolation", transaction.GetIsolationLevel().String()),
		)
	}

	queryStats := make([]*stroppy.DriverQueryStat, 0, len(transaction.GetQueries()))
	txStart := time.Now()

	for _, query := range transaction.GetQueries() {
		values := make([]any, len(query.GetParams()))

		err := d.fillParamsToValues(query, values)
		if err != nil {
			return nil, err
		}

		start := time.Now()

		_, err = d.pool.Exec(ctx, query.GetRequest(), values)
		if err != nil {
			return nil, err
		}

		queryStats = append(queryStats, &stroppy.DriverQueryStat{
			Name:         query.GetName(),
			ExecDuration: durationpb.New(time.Since(start)),
		})
	}

	return &stroppy.DriverTransactionStat{
		IsolationLevel: stroppy.TxIsolationLevel_UNSPECIFIED,
		ExecDuration:   durationpb.New(time.Since(txStart)),
		Queries:        queryStats,
	}, nil
}
