package postgres

import (
	"context"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func ptr[T any](v T) *T {
	return &v
}

type testDriver struct {
	*Driver
}

func newTestDriver(mockPool pgxmock.PgxPoolIface) (*testDriver, error) {
	return &testDriver{
		Driver: &Driver{
			logger:  logger.Global(),
			pgxPool: mockPool,
		},
	}, nil
}

func TestDriver_InsertValuesPlainQuery(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	defer mock.Close()

	drv, err := newTestDriver(mock)
	require.NoError(t, err)

	ctx := context.Background()
	descriptor := &stroppy.InsertDescriptor{
		Count:     3,
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

	// Expect 3 insert executions
	for range descriptor.GetCount() {
		mock.ExpectExec("insert into test_table").
			WithArgs(pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
	}

	stats, err := drv.InsertValues(ctx, descriptor)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Queries, 1)
	require.Equal(t, "test_table", stats.Queries[0].Name)

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
		Count:     5,
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

	// Expect one CopyFrom call with 5 rows
	mock.ExpectCopyFrom(
		[]string{"test_table"},
		[]string{"id", "name"},
	).WillReturnResult(int64(descriptor.GetCount()))

	stats, err := drv.InsertValues(ctx, descriptor)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Queries, 1)
	require.Equal(t, "test_table", stats.Queries[0].Name)

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
		Count:     10000,
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

	// Expect one CopyFrom call with 10000 rows - demonstrates streaming without memory issues
	mock.ExpectCopyFrom(
		[]string{"test_table"},
		[]string{"id", "value"},
	).WillReturnResult(int64(descriptor.Count))

	stats, err := drv.InsertValues(ctx, descriptor)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Queries, 1)
	require.Equal(t, "test_table", stats.Queries[0].Name)

	require.NoError(t, mock.ExpectationsWereMet())
}
