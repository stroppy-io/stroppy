package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/durationpb"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// InsertValues inserts multiple rows into the database based on the descriptor.
// It supports two methods:
// - PLAIN_QUERY: executes individual INSERT statements for each row
// - COPY_FROM: uses PostgreSQL's COPY protocol for fast bulk insertion
// The count parameter specifies how many rows to insert.
func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
	count int64,
) (*stroppy.DriverTransactionStat, error) {
	// Add generators for the descriptor
	unitDesc := descriptor

	err := d.builder.AddGenerators(unitDesc)
	if err != nil {
		return nil, err
	}

	txStart := time.Now()

	switch descriptor.GetMethod() {
	case stroppy.InsertMethod_PLAIN_QUERY:
		return d.insertValuesPlainQuery(ctx, descriptor, count, txStart)
	case stroppy.InsertMethod_COPY_FROM:
		return d.insertValuesCopyFrom(ctx, descriptor, count, txStart)
	default:
		d.logger.Panic("unexpected proto.InsertMethod")

		return nil, nil //nolint:nilnil // unreachable after panic
	}
}

// insertValuesPlainQuery executes multiple INSERT statements sequentially.
// Each row is inserted with a separate pgx.Exec call.
func (d *Driver) insertValuesPlainQuery(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
	count int64,
	txStart time.Time,
) (*stroppy.DriverTransactionStat, error) {
	queryStart := time.Now()

	// Execute multiple inserts
	for range count {
		transaction, err := d.GenerateNextUnit(ctx, descriptor)
		if err != nil {
			return nil, err
		}

		if len(transaction.GetQueries()) == 0 {
			continue
		}

		query := transaction.GetQueries()[0]
		values := make([]any, len(query.GetParams()))

		err = d.fillParamsToValues(query, values)
		if err != nil {
			return nil, err
		}

		_, err = d.pgxPool.Exec(ctx, query.GetRequest(), values...)
		if err != nil {
			return nil, err
		}
	}

	return &stroppy.DriverTransactionStat{
		IsolationLevel: stroppy.TxIsolationLevel_UNSPECIFIED,
		ExecDuration:   durationpb.New(time.Since(txStart)),
		Queries: []*stroppy.DriverQueryStat{{
			Name:         descriptor.GetName(),
			ExecDuration: durationpb.New(time.Since(queryStart)),
		}},
	}, nil
}

// insertValuesCopyFrom uses PostgreSQL's COPY protocol for fast bulk insertion.
// It streams values on-demand without loading all rows into memory, making it suitable
// for very large counts. Values are generated as the COPY protocol requests them.
func (d *Driver) insertValuesCopyFrom(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
	count int64,
	txStart time.Time,
) (*stroppy.DriverTransactionStat, error) {
	// Get column names
	cols := make([]string, 0, len(descriptor.GetParams()))
	for _, p := range descriptor.GetParams() {
		cols = append(cols, p.GetName())
	}

	for _, g := range descriptor.GetGroups() {
		for _, p := range g.GetParams() {
			cols = append(cols, p.GetName())
		}
	}

	queryStart := time.Now()

	_, err := d.pgxPool.CopyFrom(
		ctx,
		pgx.Identifier{descriptor.GetTableName()},
		cols,
		newStreamingCopySource(d, descriptor, count),
	)
	if err != nil {
		return nil, err
	}

	return &stroppy.DriverTransactionStat{
		IsolationLevel: stroppy.TxIsolationLevel_UNSPECIFIED,
		ExecDuration:   durationpb.New(time.Since(txStart)),
		Queries: []*stroppy.DriverQueryStat{{
			Name:         descriptor.GetName(),
			ExecDuration: durationpb.New(time.Since(queryStart)),
		}},
	}, nil
}
