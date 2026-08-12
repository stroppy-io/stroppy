package driver

import (
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/gen"
)

// InsertRequest is the typed successor to dgproto.InsertSpec: it carries
// a driver-owned [InsertMethod], a worker count, and a [gen.BatchSource]
// whose prepared partitions fill reusable typed batches. The legacy
// InsertSpec path builds a Partitionable from a protobuf generator oneof;
// this path hands a workload-authored source straight to the driver, so
// generation stays allocation-free and no protobuf is synthesized.
//
// Source, Method, and Workers are the driver's full input; the table
// name is carried separately because generation is table-agnostic.
type InsertRequest struct {
	Table   string
	Method  InsertMethod
	Workers int
	Source  gen.BatchSource
}

// ErrNilInsertRequest is returned when a driver's Insert receives a nil
// request pointer.
var ErrNilInsertRequest = errors.New("driver: nil insert request")

// ErrNilInsertSource is returned when an InsertRequest carries no source.
var ErrNilInsertSource = errors.New("driver: nil insert source")

// ErrInsertMethodNotSupported is returned by a driver's Insert when the
// request's method is not one the driver serves (see [InsertCapabilities]).
// It is the typed-path sibling of the per-driver InsertSpec sentinels.
var ErrInsertMethodNotSupported = errors.New("driver: insert method not supported")

// ValidateInsert checks the request shape shared by every driver's
// Insert entry. A nil request, a nil source, or a zero-column schema are
// rejected before driver-specific dispatch; per-driver method capability
// is validated in each driver.
func ValidateInsert(req *InsertRequest) error {
	if req == nil {
		return fmt.Errorf("%w", ErrNilInsertRequest)
	}

	if req.Source == nil {
		return fmt.Errorf("%w", ErrNilInsertSource)
	}

	if req.Source.Schema().Columns() == 0 {
		return fmt.Errorf("%w", ErrNilInsertSource)
	}

	return nil
}
