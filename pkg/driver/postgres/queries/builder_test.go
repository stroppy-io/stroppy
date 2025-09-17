package queries

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stroppy-io/stroppy-core/pkg/generate"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

func TestNewQueryBuilder_Success(t *testing.T) {
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	paramID := NewGeneratorID("test", "q1", "id")
	generator, err := generate.NewValueGenerator(42, 1, &stroppy.QueryParamDescriptor{
		Name: "id",
		GenerationRule: &stroppy.Generation_Rule{
			Type: &stroppy.Generation_Rule_Int32Rules{
				Int32Rules: &stroppy.Generation_Rules_Int32Rule{
					Constant: proto.Int32(10),
				},
			},
		},
	})
	require.NoError(t, err)
	generators.Set(paramID, generator)

	builder := &QueryBuilder{
		generators: generators,
	}
	require.NotNil(t, builder)
	require.NotNil(t, builder.generators)
}

func TestNewQueryBuilder_EmptyContext(t *testing.T) {
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	builder := &QueryBuilder{
		generators: generators,
	}
	require.NotNil(t, builder)
	require.NotNil(t, builder.generators)
}

func TestQueryBuilder_Build_Success(t *testing.T) {
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

	builder := &QueryBuilder{
		generators: generators,
	}

	unitBuildContext := &stroppy.UnitBuildContext{
		Context: buildContext,
		Unit:    step.GetUnits()[0],
	}

	ctx := context.Background()
	lg := zap.NewNop()

	transactionList, err := builder.Build(ctx, lg, unitBuildContext)
	require.NoError(t, err)
	require.NotNil(t, transactionList)
	require.Len(t, transactionList.Transactions, 1)
	require.Len(t, transactionList.Transactions[0].Queries, 1)
	require.Equal(t, int32(10), transactionList.Transactions[0].Queries[0].Params[0].GetInt32())
}

func TestQueryBuilder_BuildStream_Success(t *testing.T) {
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

	builder := &QueryBuilder{
		generators: generators,
	}

	unitBuildContext := &stroppy.UnitBuildContext{
		Context: buildContext,
		Unit:    step.GetUnits()[0],
	}

	ctx := context.Background()
	lg := zap.NewNop()

	channel := make(errchan.Chan[stroppy.DriverTransaction], 1)
	go func() {
		builder.BuildStream(ctx, lg, unitBuildContext, channel)
	}()

	transactions, err := errchan.Collect[stroppy.DriverTransaction](channel)
	require.NoError(t, err)
	require.Len(t, transactions, 1)
	require.Len(t, transactions[0].Queries, 1)
	require.Equal(t, int32(10), transactions[0].Queries[0].Params[0].GetInt32())
}

func TestQueryBuilder_Build_CreateTable(t *testing.T) {
	createTableDescriptor := &stroppy.TableDescriptor{
		Name: "test_table",
		Columns: []*stroppy.ColumnDescriptor{
			{Name: "id", SqlType: "INTEGER"},
			{Name: "name", SqlType: "VARCHAR(255)"},
		},
	}
	step := &stroppy.StepDescriptor{
		Name: "test",
		Units: []*stroppy.StepUnitDescriptor{
			{
				Type: &stroppy.StepUnitDescriptor_CreateTable{
					CreateTable: createTableDescriptor,
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

	builder := &QueryBuilder{
		generators: generators,
	}

	unitBuildContext := &stroppy.UnitBuildContext{
		Context: buildContext,
		Unit:    step.GetUnits()[0],
	}

	ctx := context.Background()
	lg := zap.NewNop()

	transactionList, err := builder.Build(ctx, lg, unitBuildContext)
	require.NoError(t, err)
	require.NotNil(t, transactionList)
	require.Len(t, transactionList.Transactions, 1)
	require.Len(t, transactionList.Transactions[0].Queries, 1)
	require.Contains(t, transactionList.Transactions[0].Queries[0].Request, "CREATE TABLE")
}

func TestQueryBuilder_Build_Transaction(t *testing.T) {
	transactionDescriptor := &stroppy.TransactionDescriptor{
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
					Transaction: transactionDescriptor,
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
	generator, err := generate.NewValueGenerator(42, 1, transactionDescriptor.GetQueries()[0].GetParams()[0])
	require.NoError(t, err)
	generators.Set(paramID, generator)

	builder := &QueryBuilder{
		generators: generators,
	}

	unitBuildContext := &stroppy.UnitBuildContext{
		Context: buildContext,
		Unit:    step.GetUnits()[0],
	}

	ctx := context.Background()
	lg := zap.NewNop()

	transactionList, err := builder.Build(ctx, lg, unitBuildContext)
	require.NoError(t, err)
	require.NotNil(t, transactionList)
	require.Len(t, transactionList.Transactions, 1)
	require.Len(t, transactionList.Transactions[0].Queries, 1)
	require.Equal(t, int32(10), transactionList.Transactions[0].Queries[0].Params[0].GetInt32())
}

func TestQueryBuilder_Build_UnknownType(t *testing.T) {
	step := &stroppy.StepDescriptor{
		Name: "test",
		Units: []*stroppy.StepUnitDescriptor{
			{
				Type: nil, // неизвестный тип
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

	builder := &QueryBuilder{
		generators: generators,
	}

	unitBuildContext := &stroppy.UnitBuildContext{
		Context: buildContext,
		Unit:    step.GetUnits()[0],
	}

	ctx := context.Background()
	lg := zap.NewNop()

	// Тестируем BuildStream напрямую, чтобы поймать панику
	channel := make(errchan.Chan[stroppy.DriverTransaction])

	require.Panics(t, func() {
		builder.BuildStream(ctx, lg, unitBuildContext, channel)
	})
}

func TestValueToPgxValue_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		val  *stroppy.Value
	}{
		{"null", &stroppy.Value{Type: &stroppy.Value_Null{}}},
		{"int32", &stroppy.Value{Type: &stroppy.Value_Int32{Int32: 42}}},
		{"uint32", &stroppy.Value{Type: &stroppy.Value_Uint32{Uint32: 42}}},
		{"int64", &stroppy.Value{Type: &stroppy.Value_Int64{Int64: 42}}},
		{"uint64", &stroppy.Value{Type: &stroppy.Value_Uint64{Uint64: 42}}},
		{"float", &stroppy.Value{Type: &stroppy.Value_Float{Float: 3.14}}},
		{"double", &stroppy.Value{Type: &stroppy.Value_Double{Double: 2.71}}},
		{"string", &stroppy.Value{Type: &stroppy.Value_String_{String_: "abc"}}},
		{"bool", &stroppy.Value{Type: &stroppy.Value_Bool{Bool: true}}},
		{"decimal", &stroppy.Value{Type: &stroppy.Value_Decimal{Decimal: &stroppy.Decimal{Value: "1.23"}}}},
		{"uuid", &stroppy.Value{Type: &stroppy.Value_Uuid{Uuid: &stroppy.Uuid{Value: uuid.NewString()}}}},
		{
			"datetime",
			&stroppy.Value{Type: &stroppy.Value_Datetime{
				Datetime: &stroppy.DateTime{Value: timestamppb.New(time.Now())},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
			builder := &QueryBuilder{
				generators: generators,
			}
			_, err := builder.ValueToPgxValue(tt.val)
			require.NoError(t, err)
		})
	}
}

func TestValueToPgxValue_Unsupported(t *testing.T) {
	val := &stroppy.Value{Type: &stroppy.Value_Struct_{Struct: &stroppy.Value_Struct{}}}

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	builder := &QueryBuilder{
		generators: generators,
	}
	_, err := builder.ValueToPgxValue(val)
	require.Error(t, err)
}

func TestValueToPgxValue_DecimalNil(t *testing.T) {
	val := &stroppy.Value{Type: &stroppy.Value_Decimal{Decimal: nil}}

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	builder := &QueryBuilder{
		generators: generators,
	}
	result, err := builder.ValueToPgxValue(val)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestValueToPgxValue_DecimalInvalid(t *testing.T) {
	val := &stroppy.Value{Type: &stroppy.Value_Decimal{Decimal: &stroppy.Decimal{Value: "invalid"}}}

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	builder := &QueryBuilder{
		generators: generators,
	}
	_, err := builder.ValueToPgxValue(val)
	require.Error(t, err)
}

func TestValueToPgxValue_UuidInvalid(t *testing.T) {
	val := &stroppy.Value{Type: &stroppy.Value_Uuid{Uuid: &stroppy.Uuid{Value: "invalid-uuid"}}}

	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	builder := &QueryBuilder{
		generators: generators,
	}
	_, err := builder.ValueToPgxValue(val)
	require.Error(t, err)
}

func TestValueToPgxValue_ReturnValues(t *testing.T) {
	generators := cmap.NewStringer[GeneratorID, generate.ValueGenerator]()
	builder := &QueryBuilder{
		generators: generators,
	}

	// Тест для int32
	int32Val := &stroppy.Value{Type: &stroppy.Value_Int32{Int32: 42}}
	result, err := builder.ValueToPgxValue(int32Val)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Тест для string
	stringVal := &stroppy.Value{Type: &stroppy.Value_String_{String_: "test"}}
	result, err = builder.ValueToPgxValue(stringVal)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Тест для bool
	boolVal := &stroppy.Value{Type: &stroppy.Value_Bool{Bool: true}}
	result, err = builder.ValueToPgxValue(boolVal)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Тест для null
	nullVal := &stroppy.Value{Type: &stroppy.Value_Null{}}
	result, err = builder.ValueToPgxValue(nullVal)
	require.NoError(t, err)
	require.Nil(t, result)
}
