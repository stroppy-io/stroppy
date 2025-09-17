package queries

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

func TestNewCreateTable_Success(t *testing.T) {
	descriptor := &stroppy.TableDescriptor{
		Name:    "t1",
		Columns: []*stroppy.ColumnDescriptor{{Name: "id", SqlType: "INT", PrimaryKey: true}},
	}
	ctx := context.Background()
	lg := zap.NewNop()

	channel := make(errchan.Chan[stroppy.DriverTransaction])
	go func() {
		NewCreateTable(ctx, lg, 42, descriptor, channel)
	}()

	transactions, err := errchan.Collect[stroppy.DriverTransaction](channel)
	require.NoError(t, err)
	require.NotEmpty(t, transactions)
	require.NotEmpty(t, transactions[0].Queries)
}

func TestNewCreateTable_Error(t *testing.T) {
	descriptor := &stroppy.TableDescriptor{
		Name:    "t1",
		Columns: nil, // нет колонок
	}
	ctx := context.Background()
	lg := zap.NewNop()

	channel := make(errchan.Chan[stroppy.DriverTransaction])
	go func() {
		NewCreateTable(ctx, lg, 42, descriptor, channel)
	}()

	transactions, err := errchan.Collect[stroppy.DriverTransaction](channel)
	require.NoError(t, err)
	require.NotEmpty(t, transactions)
	require.NotEmpty(t, transactions[0].Queries)
	require.Empty(t, transactions[0].Queries[0].Params)
}
