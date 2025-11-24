package queries

import (
	"context"
	"testing"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func TestNewTransaction_Success(t *testing.T) {
	descriptor := &stroppy.TransactionDescriptor{
		Name: "t1",
		Queries: []*stroppy.QueryDescriptor{
			{
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
	}
	// step := &stroppy.WorkloadDescriptor{
	// 	Name: "test",
	// 	Units: []*stroppy.WorkloadUnitDescriptor{
	// 		{
	// 			Descriptor_: &stroppy.UnitDescriptor{Type: &stroppy.UnitDescriptor_Transaction{
	// 				Transaction: descriptor,
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
	generator, err := generate.NewValueGenerator(42, descriptor.GetQueries()[0].GetParams()[0])
	require.NoError(t, err)
	generators.Set(paramID, generator)

	ctx := context.Background()
	lg := zap.NewNop()

	transactions, err := NewTransaction(ctx, lg, generators, descriptor)
	require.NoError(t, err)
	require.Len(t, transactions.Queries, 1)
	require.Equal(t, "SELECT * FROM t WHERE id=$1", transactions.Queries[0].Request)
	require.Equal(t, int32(10), transactions.Queries[0].Params[0].GetInt32())
}

func TestNewTransaction_Isolation(t *testing.T) {
	descriptor := &stroppy.TransactionDescriptor{
		Name:           "t1",
		IsolationLevel: stroppy.TxIsolationLevel_READ_UNCOMMITTED,
		Queries: []*stroppy.QueryDescriptor{
			{
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
	}
	// step := &stroppy.WorkloadDescriptor{
	// 	Name: "test",
	// 	Units: []*stroppy.WorkloadUnitDescriptor{
	// 		{
	// 			Descriptor_: &stroppy.UnitDescriptor{Type: &stroppy.UnitDescriptor_Transaction{
	// 				Transaction: descriptor,
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
	generator, err := generate.NewValueGenerator(42, descriptor.GetQueries()[0].GetParams()[0])
	require.NoError(t, err)
	generators.Set(paramID, generator)

	ctx := context.Background()
	lg := zap.NewNop()

	transactions, err := NewTransaction(ctx, lg, generators, descriptor)
	require.NoError(t, err)
	require.Len(t, transactions.Queries, 1)
	require.Equal(t, "SELECT * FROM t WHERE id=$1", transactions.Queries[0].Request)
	require.Equal(t, int32(10), transactions.Queries[0].Params[0].GetInt32())
}

func TestNewTransaction_WithTxParams(t *testing.T) {
	descriptor := &stroppy.TransactionDescriptor{
		Name: "t1",
		Params: []*stroppy.QueryParamDescriptor{
			{Name: "tx_id", GenerationRule: &stroppy.Generation_Rule{
				Kind: &stroppy.Generation_Rule_Int32Const{
					Int32Const: 100,
				},
			}},
		},
		Queries: []*stroppy.QueryDescriptor{
			{
				Name: "q1",
				Sql:  "SELECT * FROM t WHERE id=${tx_id}",
			},
			{
				Name: "q2",
				Sql:  "UPDATE t SET value=${value} WHERE id=${tx_id}",
				Params: []*stroppy.QueryParamDescriptor{
					{Name: "value", GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_Int32Const{
							Int32Const: 50,
						},
					}},
				},
			},
		},
	}

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	// Transaction-level param generator
	txParamID := NewGeneratorID("t1", "tx_id")
	txGenerator, err := generate.NewValueGenerator(42, descriptor.GetParams()[0])
	require.NoError(t, err)
	generators.Set(txParamID, txGenerator)

	// Query-level param generator
	queryParamID := NewGeneratorID("q2", "value")
	queryGenerator, err := generate.NewValueGenerator(42, descriptor.GetQueries()[1].GetParams()[0])
	require.NoError(t, err)
	generators.Set(queryParamID, queryGenerator)

	ctx := context.Background()
	lg := zap.NewNop()

	transaction, err := NewTransaction(ctx, lg, generators, descriptor)
	require.NoError(t, err)
	require.Len(t, transaction.Queries, 2)

	// First query uses only tx param
	require.Equal(t, "SELECT * FROM t WHERE id=$1", transaction.Queries[0].Request)
	require.Len(t, transaction.Queries[0].Params, 1)
	require.Equal(t, int32(100), transaction.Queries[0].Params[0].GetInt32())

	// Second query uses query param first, then tx param
	require.Equal(t, "UPDATE t SET value=$1 WHERE id=$2", transaction.Queries[1].Request)
	require.Len(t, transaction.Queries[1].Params, 2)
	require.Equal(t, int32(50), transaction.Queries[1].Params[0].GetInt32())
	require.Equal(t, int32(100), transaction.Queries[1].Params[1].GetInt32())
}

func TestNewTransaction_WithTxGroups(t *testing.T) {
	descriptor := &stroppy.TransactionDescriptor{
		Name: "t1",
		Groups: []*stroppy.QueryParamGroup{
			{
				Name: "group1",
				Params: []*stroppy.QueryParamDescriptor{
					{Name: "tx_id", GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_Int32Const{
							Int32Const: 200,
						},
					}},
				},
			},
		},
		Queries: []*stroppy.QueryDescriptor{
			{
				Name: "q1",
				Sql:  "SELECT * FROM t WHERE id=${tx_id}",
			},
		},
	}

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	// Transaction-level group generator - groups generate params from the group
	txGroupID := NewGeneratorID("t1", "group1")
	txGroupGenerator, err := generate.NewValueGenerator(42, &stroppy.QueryParamDescriptor{
		Name: "group1",
		GenerationRule: &stroppy.Generation_Rule{
			Kind: &stroppy.Generation_Rule_Int32Const{
				Int32Const: 200,
			},
		},
	})
	require.NoError(t, err)
	generators.Set(txGroupID, txGroupGenerator)

	ctx := context.Background()
	lg := zap.NewNop()

	transaction, err := NewTransaction(ctx, lg, generators, descriptor)
	require.NoError(t, err)
	require.Len(t, transaction.Queries, 1)

	require.Equal(t, "SELECT * FROM t WHERE id=$1", transaction.Queries[0].Request)
	require.Len(t, transaction.Queries[0].Params, 1)
	require.Equal(t, int32(200), transaction.Queries[0].Params[0].GetInt32())
}

func TestNewTransaction_QueryParamOverridesTxParam(t *testing.T) {
	descriptor := &stroppy.TransactionDescriptor{
		Name: "t1",
		Params: []*stroppy.QueryParamDescriptor{
			{Name: "id", GenerationRule: &stroppy.Generation_Rule{
				Kind: &stroppy.Generation_Rule_Int32Const{
					Int32Const: 100,
				},
			}},
		},
		Queries: []*stroppy.QueryDescriptor{
			{
				Name: "q1",
				Sql:  "SELECT * FROM t WHERE id=${id}",
				Params: []*stroppy.QueryParamDescriptor{
					{Name: "id", GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_Int32Const{
							Int32Const: 999,
						},
					}},
				},
			},
		},
	}

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()

	// Transaction-level param
	txParamID := NewGeneratorID("t1", "id")
	txGenerator, err := generate.NewValueGenerator(42, descriptor.GetParams()[0])
	require.NoError(t, err)
	generators.Set(txParamID, txGenerator)

	// Query-level param with same name
	queryParamID := NewGeneratorID("q1", "id")
	queryGenerator, err := generate.NewValueGenerator(42, descriptor.GetQueries()[0].GetParams()[0])
	require.NoError(t, err)
	generators.Set(queryParamID, queryGenerator)

	ctx := context.Background()
	lg := zap.NewNop()

	transaction, err := NewTransaction(ctx, lg, generators, descriptor)
	require.NoError(t, err)
	require.Len(t, transaction.Queries, 1)

	// Query param should override tx param (value should be 999, not 100)
	require.Equal(t, "SELECT * FROM t WHERE id=$1", transaction.Queries[0].Request)
	require.Len(t, transaction.Queries[0].Params, 1)
	require.Equal(t, int32(999), transaction.Queries[0].Params[0].GetInt32())
}
