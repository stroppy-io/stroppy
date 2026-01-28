package queries

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

var (
	ErrUnsupportedType  = errors.New("unsupported value type")
	ErrUnknownQueryType = errors.New("unknown query type")
)

type QueryBuilder struct {
	generators Generators
	lg         *zap.Logger
	insert     *stroppy.InsertDescriptor
}

func NewQueryBuilder(
	lg *zap.Logger,
	seed uint64,
	insert *stroppy.InsertDescriptor,
) (*QueryBuilder, error) {
	gens, err := collectInsertGenerators(seed, insert)
	if err != nil {
		return nil, fmt.Errorf("add generators for unit :%w", err)
	}

	return &QueryBuilder{
		generators: gens,
		lg:         lg,
		insert:     insert,
	}, nil
}

func (q *QueryBuilder) Build() (string, []any, error) {
	return NewInsertQuery(q.lg, q.generators, q.insert)
}

func ValueToPgxValue(value *stroppy.Value) (any, error) {
	switch typed := value.GetType().(type) {
	case *stroppy.Value_Null:
		return nil, nil //nolint: nilnil               // allow to set nil in db
	case *stroppy.Value_Int32:
		return typed.Int32, nil
	case *stroppy.Value_Uint32:
		return typed.Uint32, nil
	case *stroppy.Value_Int64:
		return typed.Int64, nil
	case *stroppy.Value_Uint64:
		return typed.Uint64, nil
	case *stroppy.Value_Float:
		return typed.Float, nil
	case *stroppy.Value_Double:
		return typed.Double, nil
	case *stroppy.Value_String_:
		return typed.String_, nil
	case *stroppy.Value_Bool:
		return typed.Bool, nil
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
