package driver

import (
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// InsertMethod names a row-insertion strategy. It is the driver-owned
// successor to the frozen dgproto.InsertMethod enum: the legacy relational
// InsertSpec path still receives a dgproto.InsertMethod (the protobuf is not
// extended), but the typed Driver.Insert path uses this Go-native enum. The
// two are converted at the boundary by MethodFromProto / MethodToProto.
//
// The method describes how a driver lands rows, not how a generator stores
// them: NATIVE is a driver-specific bulk primitive (COPY, BulkUpsert, …),
// PLAIN_BULK is a multi-row INSERT, PLAIN_QUERY is one INSERT per row, and
// COLUMNAR is a single-array-per-column protocol (e.g. unnest). A driver
// advertises the methods it serves via [InsertCapabilities].
type InsertMethod int

//nolint:revive // method enum starts at 1 so the zero value is "unspecified".
const (
	InsertPlainQuery InsertMethod = iota + 1
	InsertPlainBulk
	InsertColumnar
	InsertNative
)

// String returns the stable, lowercase method name used in CLI strings and
// probe output. It matches the legacy authoring strings ("plain_query",
// "plain_bulk", "columnar", "native").
func (m InsertMethod) String() string {
	switch m {
	case InsertPlainQuery:
		return "plain_query"
	case InsertPlainBulk:
		return "plain_bulk"
	case InsertColumnar:
		return "columnar"
	case InsertNative:
		return "native"
	default:
		return fmt.Sprintf("insert_method(%d)", int(m))
	}
}

// ErrUnknownInsertMethod is returned by ParseInsertMethod for an unrecognized
// method string.
var ErrUnknownInsertMethod = errors.New("unknown insert method")

// ParseInsertMethod resolves an authoring string to a driver InsertMethod.
// The empty string selects plain_query, matching the legacy default.
func ParseInsertMethod(s string) (InsertMethod, error) {
	switch s {
	case "", "plain_query":
		return InsertPlainQuery, nil
	case "plain_bulk":
		return InsertPlainBulk, nil
	case "columnar":
		return InsertColumnar, nil
	case "native":
		return InsertNative, nil
	default:
		return 0, fmt.Errorf("%w %q", ErrUnknownInsertMethod, s)
	}
}

// MethodFromProto converts a legacy dgproto.InsertMethod to a driver
// InsertMethod. Unknown proto values map to the zero InsertMethod.
func MethodFromProto(p dgproto.InsertMethod) InsertMethod {
	switch p {
	case dgproto.InsertMethod_PLAIN_QUERY:
		return InsertPlainQuery
	case dgproto.InsertMethod_PLAIN_BULK:
		return InsertPlainBulk
	case dgproto.InsertMethod_COLUMNAR:
		return InsertColumnar
	case dgproto.InsertMethod_NATIVE:
		return InsertNative
	default:
		return 0
	}
}

// MethodToProto converts a driver InsertMethod back to the legacy protobuf
// enum, for the InsertSpec compatibility path that still runs during
// migration.
func MethodToProto(m InsertMethod) dgproto.InsertMethod {
	switch m {
	case InsertPlainQuery:
		return dgproto.InsertMethod_PLAIN_QUERY
	case InsertPlainBulk:
		return dgproto.InsertMethod_PLAIN_BULK
	case InsertColumnar:
		return dgproto.InsertMethod_COLUMNAR
	case InsertNative:
		return dgproto.InsertMethod_NATIVE
	default:
		return dgproto.InsertMethod_PLAIN_QUERY
	}
}
