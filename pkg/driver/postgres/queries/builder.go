package queries

import (
	"context"
	"errors"

	"github.com/google/uuid"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

var (
	ErrUnsupportedType  = errors.New("unsupported value type")
	ErrUnknownQueryType = errors.New("unknown query type")
)

type QueryBuilder struct {
	generators Generators
}

func NewQueryBuilder(runContext *stroppy.StepContext) (*QueryBuilder, error) {
	gens, err := CollectStepGenerators(runContext)
	if err != nil {
		return nil, err
	}

	return &QueryBuilder{
		generators: gens,
	}, nil
}

func (q *QueryBuilder) BuildStream(
	ctx context.Context,
	logger *zap.Logger,
	buildQueriesContext *stroppy.UnitBuildContext,
	channel errchan.Chan[stroppy.DriverTransaction],
) {
	q.internalBuild(ctx, logger, buildQueriesContext, channel)
}

func (q *QueryBuilder) Build(
	ctx context.Context,
	logger *zap.Logger,
	buildQueriesContext *stroppy.UnitBuildContext,
) (*stroppy.DriverTransactionList, error) {
	channel := make(errchan.Chan[stroppy.DriverTransaction])
	go func() {
		q.internalBuild(ctx, logger, buildQueriesContext, channel)
	}()

	transactions, err := errchan.CollectCtx[stroppy.DriverTransaction](ctx, channel)
	if err != nil {
		return nil, err
	}

	return &stroppy.DriverTransactionList{
		Transactions: transactions,
	}, nil
}

func (q *QueryBuilder) internalBuild(
	ctx context.Context,
	logger *zap.Logger,
	buildQueriesContext *stroppy.UnitBuildContext,
	channel errchan.Chan[stroppy.DriverTransaction],
) {
	switch buildQueriesContext.GetUnit().GetType().(type) {
	case *stroppy.StepUnitDescriptor_CreateTable:
		NewCreateTable(
			ctx,
			logger,
			buildQueriesContext.GetContext().GetGlobalConfig().GetRun().GetSeed(),
			buildQueriesContext.GetUnit().GetCreateTable(),
			channel,
		)
	case *stroppy.StepUnitDescriptor_Query:
		NewQuery(
			ctx,
			logger,
			q.generators,
			buildQueriesContext.GetContext(),
			buildQueriesContext.GetUnit().GetQuery(),
			channel,
		)
	case *stroppy.StepUnitDescriptor_Transaction:
		NewTransaction(
			ctx,
			logger,
			q.generators,
			buildQueriesContext.GetContext(),
			buildQueriesContext.GetUnit().GetTransaction(),
			channel,
		)
	default:
		panic(ErrUnknownQueryType)
	}
}

func (q *QueryBuilder) ValueToPgxValue(value *stroppy.Value) (any, error) {
	switch value.GetType().(type) {
	case *stroppy.Value_Null:
		return nil, nil //nolint: nilnil // allow to set nil in db
	case *stroppy.Value_Int32:
		return pgtype.Int4{
			Valid: true,
			Int32: value.GetInt32(),
		}, nil
	case *stroppy.Value_Uint32:
		return pgtype.Uint32{
			Valid:  true,
			Uint32: value.GetUint32(),
		}, nil
	case *stroppy.Value_Int64:
		return &pgtype.Int8{
			Valid: true,
			Int64: value.GetInt64(),
		}, nil
	case *stroppy.Value_Uint64:
		return &pgtype.Uint64{
			Valid:  true,
			Uint64: value.GetUint64(),
		}, nil
	case *stroppy.Value_Float:
		return &pgtype.Float4{
			Valid:   true,
			Float32: value.GetFloat(),
		}, nil
	case *stroppy.Value_Double:
		return &pgtype.Float8{
			Valid:   true,
			Float64: value.GetDouble(),
		}, nil
	case *stroppy.Value_String_:
		return &pgtype.Text{
			Valid:  true,
			String: value.GetString_(),
		}, nil
	case *stroppy.Value_Bool:
		return &pgtype.Bool{
			Valid: true,
			Bool:  value.GetBool(),
		}, nil
	case *stroppy.Value_Decimal:
		if value.GetDecimal() == nil {
			return &pgxdecimal.NullDecimal{}, nil
		}

		dec, err := decimal.NewFromString(value.GetDecimal().GetValue())
		if err != nil {
			return nil, err
		}

		return pgxdecimal.Decimal(dec), nil
	case *stroppy.Value_Uuid:
		uuidVal, err := uuid.Parse(value.GetUuid().GetValue())
		if err != nil {
			return nil, err
		}

		return &pgtype.UUID{
			Valid: true,
			Bytes: uuidVal,
		}, nil
	case *stroppy.Value_Datetime:
		return &pgtype.Timestamptz{
			Valid: true,
			Time:  value.GetDatetime().GetValue().AsTime(),
		}, nil
	default:
		return nil, ErrUnsupportedType
	}
}
