package queries

import (
	"context"
	"testing"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

func TestNewQuery_Success(t *testing.T) {
	descriptor := &stroppy.QueryDescriptor{
		Name: "q1",
		Sql:  "SELECT * FROM t WHERE id=${id}",
		Params: []*stroppy.QueryParamDescriptor{
			{Name: "id", GenerationRule: &stroppy.Generation_Rule{
				Kind: &stroppy.Generation_Rule_Int32Const{
					Int32Const: 10,
				},
			}},
		},
	}
	// step := &stroppy.WorkloadDescriptor{
	// 	Name: "test",
	// 	Units: []*stroppy.WorkloadUnitDescriptor{
	// 		{
	// 			Descriptor_: &stroppy.UnitDescriptor{Type: &stroppy.UnitDescriptor_Query{
	// 				Query: descriptor,
	// 			}},
	// 		},
	// 	},
	// }
	// buildContext := &stroppy.StepContext{
	// 	GlobalConfig: &stroppy.Config{
	// 		Run: &stroppy.RunConfig{
	// 			Seed: 42,
	// 		},
	// 	},
	// 	Step: step,
	// }

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	paramID := NewGeneratorID("q1", "id")
	generator, err := generate.NewValueGenerator(42, descriptor.GetParams()[0])
	require.NoError(t, err)
	generators.Set(paramID, generator)

	ctx := context.Background()
	lg := zap.NewNop()

	transactions, err := NewQuery(ctx, lg, generators, descriptor)
	require.NoError(t, err)
	require.Len(t, transactions.Queries, 1)
	require.Equal(t, int32(10), transactions.Queries[0].Params[0].GetInt32())
}

func TestNewQuery_Error(t *testing.T) {
	descriptor := &stroppy.QueryDescriptor{
		Name:   "q1",
		Sql:    "SELECT * FROM t WHERE id=${id}",
		Params: []*stroppy.QueryParamDescriptor{}, // нет генераторов
	}
	// step := &stroppy.WorkloadDescriptor{
	// 	Name: "test",
	// 	Units: []*stroppy.WorkloadUnitDescriptor{
	// 		{
	// 			Descriptor_: &stroppy.UnitDescriptor{Type: &stroppy.UnitDescriptor_Query{
	// 				Query: descriptor,
	// 			}},
	// 		},
	// 	},
	// }
	// buildContext := &stroppy.StepContext{
	// 	GlobalConfig: &stroppy.Config{
	// 		Run: &stroppy.RunConfig{
	// 			Seed: 42,
	// 		},
	// 	},
	// 	Step: step,
	// }

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	ctx := context.Background()
	lg := zap.NewNop()

	transactions, err := NewQuery(ctx, lg, generators, descriptor)
	require.NoError(t, err)
	// require.Equal(t, int32(10), transactions.Queries[0].Params[0].GetInt32())
	require.Len(t, transactions.Queries, 1)
}

func TestNewQuerySync_Success(t *testing.T) {
	descriptor := &stroppy.QueryDescriptor{
		Name: "q1",
		Sql:  "SELECT * FROM t WHERE id=${id}",
		Params: []*stroppy.QueryParamDescriptor{
			{Name: "id", GenerationRule: &stroppy.Generation_Rule{
				Kind: &stroppy.Generation_Rule_Int32Const{
					Int32Const: 10,
				},
			}},
		},
	}
	// step := &stroppy.WorkloadDescriptor{
	// 	Name: "test",
	// 	Units: []*stroppy.WorkloadUnitDescriptor{
	// 		{
	// 			Descriptor_: &stroppy.UnitDescriptor{Type: &stroppy.UnitDescriptor_Query{
	// 				Query: descriptor,
	// 			}},
	// 		},
	// 	},
	// }
	// buildContext := &stroppy.StepContext{
	// 	GlobalConfig: &stroppy.Config{
	// 		Run: &stroppy.RunConfig{
	// 			Seed: 42,
	// 		},
	// 	},
	// 	Step: step,
	// }

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	paramID := NewGeneratorID("q1", "id")
	generator, err := generate.NewValueGenerator(42, descriptor.GetParams()[0])
	require.NoError(t, err)
	generators.Set(paramID, generator)

	ctx := context.Background()
	lg := zap.NewNop()

	transaction, err := NewQuery(ctx, lg, generators, descriptor)
	require.NoError(t, err)
	require.NotNil(t, transaction)
	require.Len(t, transaction.Queries, 1)
	require.Equal(t, int32(10), transaction.Queries[0].Params[0].GetInt32())
}

func Test_interpolateSQL(t *testing.T) {
	type args struct {
		descriptor *stroppy.QueryDescriptor
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "simple",
			args: args{
				descriptor: &stroppy.QueryDescriptor{
					Name: "q1",
					Sql:  "SELECT * FROM t WHERE id=${id}",
					Params: []*stroppy.QueryParamDescriptor{
						{Name: "id", GenerationRule: &stroppy.Generation_Rule{
							Kind: &stroppy.Generation_Rule_Int32Const{
								Int32Const: 10,
							},
						}},
					},
				},
			},
			want: "SELECT * FROM t WHERE id=$1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interpolateSQL(tt.args.descriptor); got != tt.want {
				t.Errorf("interpolateSQL() = %v, want %v", got, tt.want)
			}
		})
	}
}
