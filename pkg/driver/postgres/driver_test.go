package postgres

import (
	"context"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy-core/pkg/logger"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
)

type testDriver struct {
	*Driver
}

func newTestDriver(mockPool pgxmock.PgxPoolIface) *testDriver {
	return &testDriver{
		Driver: &Driver{
			logger:  logger.Global(),
			pgxPool: mockPool,
		},
	}
}

func TestDriver_RunQuery(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	drv := newTestDriver(mock)

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

	err = drv.RunTransaction(ctx, query)
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}
