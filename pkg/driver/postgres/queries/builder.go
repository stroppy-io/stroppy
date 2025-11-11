package queries

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5/pgtype"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/generate"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

var (
	ErrUnsupportedType  = errors.New("unsupported value type")
	ErrUnknownQueryType = errors.New("unknown query type")
)

type QueryBuilder struct {
	unitNamesSet cmap.ConcurrentMap[string, struct{}]
	generators   Generators
	seed         uint64
	mutex        *sync.Mutex
}

func NewQueryBuilder(seed uint64) (*QueryBuilder, error) {
	return &QueryBuilder{
		unitNamesSet: cmap.New[struct{}](),
		generators:   cmap.NewStringer[GeneratorID, generate.ValueGenerator](),
		seed:         seed,
		mutex:        &sync.Mutex{},
	}, nil
}

func (q *QueryBuilder) AddGenerators(unit *stroppy.UnitDescriptor) error {
	name, err := unitName(unit)
	if err != nil {
		return err
	}
	// Lock to ensure thread-safe check-and-add operation for unit generators
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.unitNamesSet.Has(name) {
		return nil
	}
	gens, err := collectUnitGenerators(unit, q.seed)
	if err != nil {
		return fmt.Errorf("add generators for unit :%w", err)
	}
	q.generators.MSet(gens.Items())
	q.unitNamesSet.Set(name, struct{}{})
	return nil
}

var ErrNoSubtype = errors.New("no subtype set in UnitDescriptor")

func unitName(unit *stroppy.UnitDescriptor) (string, error) {
	if create := unit.GetCreateTable(); create != nil {
		return create.GetName(), nil
	}
	if insert := unit.GetInsert(); insert != nil {
		return insert.GetName(), nil
	}
	if query := unit.GetQuery(); query != nil {
		return query.GetName(), nil
	}
	if transaction := unit.GetTransaction(); transaction != nil {
		return transaction.GetName(), nil
	}
	return "", ErrNoSubtype
}

func (q *QueryBuilder) Build(
	ctx context.Context,
	logger *zap.Logger,
	unit *stroppy.UnitDescriptor,
) (*stroppy.DriverTransaction, error) {
	return q.internalBuild(ctx, logger, unit)
}

func (q *QueryBuilder) internalBuild(
	ctx context.Context,
	lg *zap.Logger,
	descriptor *stroppy.UnitDescriptor,
) (*stroppy.DriverTransaction, error) {
	if qry := descriptor.GetQuery(); qry != nil {
		return NewQuery(ctx, lg, q.generators, qry)
	}
	if tx := descriptor.GetTransaction(); tx != nil {
		return NewTransaction(ctx, lg, q.generators, tx)
	}
	if ins := descriptor.GetInsert(); ins != nil {
		return NewInsertQuery(ctx, lg, q.generators, ins)
	}
	if ct := descriptor.GetCreateTable(); ct != nil {
		return NewCreateTable(lg, ct)
	}
	return nil, ErrUnknownQueryType
}

func (q *QueryBuilder) ValueToPgxValue(value *stroppy.Value) (any, error) {
	switch typed := value.GetType().(type) {
	case *stroppy.Value_Null:
		return nil, nil //nolint: nilnil               // allow to set nil in db
	case *stroppy.Value_Int32:
		return typed.Int32, nil // &pgtype.Int4{Valid: true, Int32: typed.Int32}, nil
	case *stroppy.Value_Uint32:
		return typed.Uint32, nil // &pgtype.Uint32{Valid: true, Uint32: typed.Uint32}, nil
	case *stroppy.Value_Int64:
		return typed.Int64, nil // &pgtype.Int8{Valid: true, Int64: typed.Int64}, nil
	case *stroppy.Value_Uint64:
		return typed.Uint64, nil // &pgtype.Uint64{Valid: true, Uint64: typed.Uint64}, nil
	case *stroppy.Value_Float:
		return typed.Float, nil // &pgtype.Float4{Valid: true, Float32: typed.Float}, nil
	case *stroppy.Value_Double:
		return typed.Double, nil // &pgtype.Float8{Valid: true, Float64: typed.Double}, nil
	case *stroppy.Value_String_:
		return typed.String_, nil // &pgtype.Text{Valid: true, String: typed.String_}, nil
	case *stroppy.Value_Bool:
		return typed.Bool, nil // &pgtype.Bool{Valid: true, Bool: typed.Bool}, nil
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

		return &pgtype.UUID{Valid: true, Bytes: uuidVal}, nil
	case *stroppy.Value_Datetime:
		return &pgtype.Timestamptz{
			Valid: true,
			Time:  value.GetDatetime().GetValue().AsTime(),
		}, nil
	default:
		return nil, ErrUnsupportedType
	}
}
