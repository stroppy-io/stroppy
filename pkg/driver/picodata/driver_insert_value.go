package picodata

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

var ErrCopyFromUnsupported = errors.New("CopyFrom is not supported in Picodata yet")

// InsertValues inserts multiple rows into the database based on the descriptor.
// It supports two methods:
// - PLAIN_QUERY: executes individual INSERT statements for each row
// - COPY_FROM: unsupported
// The count parameter specifies how many rows to insert.
func (d *Driver) InsertValues(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
	count int64,
) (*stroppy.DriverTransactionStat, error) {
	// Add generators for the descriptor
	unitDesc := &stroppy.UnitDescriptor{
		Type: &stroppy.UnitDescriptor_Insert{
			Insert: descriptor,
		},
	}

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
		transaction, err := d.GenerateNextUnit(ctx, &stroppy.UnitDescriptor{
			Type: &stroppy.UnitDescriptor_Insert{
				Insert: descriptor,
			},
		})
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

		_, err = d.pool.Exec(ctx, query.GetRequest(), values...)
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

func (d *Driver) insertValuesCopyFrom(
	ctx context.Context,
	descriptor *stroppy.InsertDescriptor,
	count int64,
	txStart time.Time,
) (*stroppy.DriverTransactionStat, error) {
	return nil, ErrCopyFromUnsupported
}
