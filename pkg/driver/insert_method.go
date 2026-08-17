package driver

import (
	"errors"
	"fmt"
)

// InsertMethod names a row-insertion strategy. The typed Driver.Insert path
// uses this Go-native enum; a driver advertises the methods it serves via
// [InsertCapabilities].
//
// The method describes how a driver lands rows, not how a generator stores
// them: NATIVE is a driver-specific bulk primitive (COPY, BulkUpsert, …),
// PLAIN_BULK is a multi-row INSERT, PLAIN_QUERY is one INSERT per row, and
// COLUMNAR is a single-array-per-column protocol (e.g. unnest).
type InsertMethod int

//nolint:revive // method enum starts at 1 so the zero value is "unspecified".
const (
	InsertPlainQuery InsertMethod = iota + 1
	InsertPlainBulk
	InsertColumnar
	InsertNative
)

// String returns the stable, lowercase method name used in CLI strings and
// probe output ("plain_query", "plain_bulk", "columnar", "native").
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
