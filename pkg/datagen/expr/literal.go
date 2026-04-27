package expr

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// evalLiteral returns the Go-typed value stored in the Literal oneof.
// Timestamps are surfaced as time.Time via timestamppb.Timestamp.AsTime.
// The Null arm returns (nil, nil) — nil is the row-scratch representation
// of SQL NULL, propagated to drivers untouched.
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
	case *dgproto.Literal_Null:
		// Go nil is the row-scratch representation of SQL NULL; the
		// nil-error return here is load-bearing and intentional.
		return nil, nil //nolint:nilnil // SQL NULL is a valid value, not an error
	default:
		return nil, fmt.Errorf("%w: literal %T", ErrBadExpr, value)
	}
}
