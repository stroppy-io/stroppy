package queries

import (
	"context"
	"testing"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy-core/pkg/generate"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

func TestNewTransaction_Success(t *testing.T) {
	descriptor := &stroppy.TransactionDescriptor{
		Name: "t1",
		Queries: []*stroppy.QueryDescriptor{
			{
				Name: "q1",
				Sql:  "SELECT * FROM t WHERE id=${id}",
				Params: []*stroppy.QueryParamDescriptor{{Name: "id", GenerationRule: &stroppy.Generation_Rule{
					Type: &stroppy.Generation_Rule_Int32Rules{
						Int32Rules: &stroppy.Generation_Rules_Int32Rule{
							Constant: proto.Int32(10),
						},
					},
				}}},
				Count: 1,
			},
		},
	}
	step := &stroppy.StepDescriptor{
		Name: "test",
		Units: []*stroppy.StepUnitDescriptor{
			{
				Type: &stroppy.StepUnitDescriptor_Transaction{
					Transaction: descriptor,
				},
			},
		},
	}
	buildContext := &stroppy.StepContext{
		GlobalConfig: &stroppy.Config{
			Run: &stroppy.RunConfig{
				Seed: 42,
			},
		},
		Step: step,
	}

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	paramID := NewGeneratorID("test", "q1", "id")
	generator, err := generate.NewValueGenerator(42, 1, descriptor.GetQueries()[0].GetParams()[0])
	require.NoError(t, err)
	generators.Set(paramID, generator)

	ctx := context.Background()
	lg := zap.NewNop()

	channel := make(errchan.Chan[stroppy.DriverTransaction], 1)
	go func() {
		NewTransaction(ctx, lg, generators, buildContext, descriptor, channel)
	}()

	transactions, err := errchan.Collect[stroppy.DriverTransaction](channel)
	require.NoError(t, err)
	require.Len(t, transactions, 1)
	require.Len(t, transactions[0].Queries, 1)
	require.Equal(t, "SELECT * FROM t WHERE id=$1", transactions[0].Queries[0].Request)
	require.Equal(t, int32(10), transactions[0].Queries[0].Params[0].GetInt32())
}

func TestNewTransaction_Isolation(t *testing.T) {
	descriptor := &stroppy.TransactionDescriptor{
		Name:           "t1",
		IsolationLevel: stroppy.TxIsolationLevel_TX_ISOLATION_LEVEL_READ_UNCOMMITTED,
		Queries: []*stroppy.QueryDescriptor{
			{
				Name: "q1",
				Sql:  "SELECT * FROM t WHERE id=${id}",
				Params: []*stroppy.QueryParamDescriptor{{Name: "id", GenerationRule: &stroppy.Generation_Rule{
					Type: &stroppy.Generation_Rule_Int32Rules{
						Int32Rules: &stroppy.Generation_Rules_Int32Rule{
							Constant: proto.Int32(10),
						},
					},
				}}},
				Count: 1,
			},
		},
	}
	step := &stroppy.StepDescriptor{
		Name: "test",
		Units: []*stroppy.StepUnitDescriptor{
			{
				Type: &stroppy.StepUnitDescriptor_Transaction{
					Transaction: descriptor,
				},
			},
		},
	}
	buildContext := &stroppy.StepContext{
		GlobalConfig: &stroppy.Config{
			Run: &stroppy.RunConfig{
				Seed: 42,
			},
		},
		Step: step,
	}

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	paramID := NewGeneratorID("test", "q1", "id")
	generator, err := generate.NewValueGenerator(42, 1, descriptor.GetQueries()[0].GetParams()[0])
	require.NoError(t, err)
	generators.Set(paramID, generator)

	ctx := context.Background()
	lg := zap.NewNop()

	channel := make(errchan.Chan[stroppy.DriverTransaction])
	go func() {
		NewTransaction(ctx, lg, generators, buildContext, descriptor, channel)
	}()

	transactions, err := errchan.Collect[stroppy.DriverTransaction](channel)
	require.NoError(t, err)
	require.Len(t, transactions, 1)
	require.Len(t, transactions[0].Queries, 1)
	require.Equal(t, "SELECT * FROM t WHERE id=$1", transactions[0].Queries[0].Request)
	require.Equal(t, int32(10), transactions[0].Queries[0].Params[0].GetInt32())
}
