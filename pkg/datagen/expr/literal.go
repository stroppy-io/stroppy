package expr

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// evalLiteral returns the Go-typed value stored in the Literal oneof.
// Timestamps are surfaced as time.Time via timestamppb.Timestamp.AsTime.
func evalLiteral(lit *dgproto.Literal) (any, error) {
	if lit == nil {
		return nil, fmt.Errorf("%w: nil literal", ErrBadExpr)
	}

	switch value := lit.GetValue().(type) {
	case *dgproto.Literal_Int64:
		return lit.GetInt64(), nil
	case *dgproto.Literal_Double:
		return lit.GetDouble(), nil
	case *dgproto.Literal_String_:
		return lit.GetString_(), nil
	case *dgproto.Literal_Bool:
		return lit.GetBool(), nil
	case *dgproto.Literal_Bytes:
		return lit.GetBytes(), nil
	case *dgproto.Literal_Timestamp:
		return lit.GetTimestamp().AsTime(), nil
	default:
		return nil, fmt.Errorf("%w: literal %T", ErrBadExpr, value)
	}
}
