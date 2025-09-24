package queries

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

func TestNewCreateTable_Success(t *testing.T) {
	descriptor := &stroppy.TableDescriptor{
		Name:    "t1",
		Columns: []*stroppy.ColumnDescriptor{{Name: "id", SqlType: "INT", PrimaryKey: true}},
	}
	// ctx := context.Background()
	lg := zap.NewNop()

	transactions, err := NewCreateTable(lg, descriptor)
	require.NoError(t, err)
	require.NotEmpty(t, transactions.Queries)
}

func TestNewCreateTable_Error(t *testing.T) {
	descriptor := &stroppy.TableDescriptor{
		Name:    "t1",
		Columns: nil, // нет колонок
	}
	// ctx := context.Background()
	lg := zap.NewNop()

	transactions, err := NewCreateTable(lg, descriptor)
	require.NoError(t, err)
	require.NotEmpty(t, transactions.Queries)
	require.Empty(t, transactions.Queries[0].Params)
}
