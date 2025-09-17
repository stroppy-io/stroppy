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

func TestNewQuery_Success(t *testing.T) {
	descriptor := &stroppy.QueryDescriptor{
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
	}
	step := &stroppy.StepDescriptor{
		Name: "test",
		Units: []*stroppy.StepUnitDescriptor{
			{
				Type: &stroppy.StepUnitDescriptor_Query{
					Query: descriptor,
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
	generator, err := generate.NewValueGenerator(42, 1, descriptor.GetParams()[0])
	require.NoError(t, err)
	generators.Set(paramID, generator)

	ctx := context.Background()
	lg := zap.NewNop()

	channel := make(errchan.Chan[stroppy.DriverTransaction], 1)
	go func() {
		NewQuery(ctx, lg, generators, buildContext, descriptor, channel)
	}()

	transactions, err := errchan.Collect[stroppy.DriverTransaction](channel)
	require.NoError(t, err)
	require.Len(t, transactions, 1)
	require.Len(t, transactions[0].Queries, 1)
	require.Equal(t, int32(10), transactions[0].Queries[0].Params[0].GetInt32())
}

func TestNewQuery_Error(t *testing.T) {
	descriptor := &stroppy.QueryDescriptor{
		Name:   "q1",
		Sql:    "SELECT * FROM t WHERE id=${id}",
		Params: []*stroppy.QueryParamDescriptor{}, // нет генераторов
		Count:  1,
	}
	step := &stroppy.StepDescriptor{
		Name: "test",
		Units: []*stroppy.StepUnitDescriptor{
			{
				Type: &stroppy.StepUnitDescriptor_Query{
					Query: descriptor,
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

	ctx := context.Background()
	lg := zap.NewNop()

	channel := make(errchan.Chan[stroppy.DriverTransaction])
	go func() {
		NewQuery(ctx, lg, generators, buildContext, descriptor, channel)
	}()

	transactions, err := errchan.Collect[stroppy.DriverTransaction](channel)
	require.NoError(t, err)
	require.Len(t, transactions, 1)
	require.Len(t, transactions[0].Queries, 1)
	require.Empty(t, transactions[0].Queries[0].Params)
}

func TestNewQuerySync_Success(t *testing.T) {
	descriptor := &stroppy.QueryDescriptor{
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
	}
	step := &stroppy.StepDescriptor{
		Name: "test",
		Units: []*stroppy.StepUnitDescriptor{
			{
				Type: &stroppy.StepUnitDescriptor_Query{
					Query: descriptor,
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
	generator, err := generate.NewValueGenerator(42, 1, descriptor.GetParams()[0])
	require.NoError(t, err)
	generators.Set(paramID, generator)

	ctx := context.Background()
	lg := zap.NewNop()

	transaction, err := NewQuerySync(ctx, lg, generators, buildContext, descriptor)
	require.NoError(t, err)
	require.NotNil(t, transaction)
	require.Len(t, transaction.Queries, 1)
	require.Equal(t, int32(10), transaction.Queries[0].Params[0].GetInt32())
}
