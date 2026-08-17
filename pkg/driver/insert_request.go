package driver

import (
	"errors"

	"github.com/stroppy-io/stroppy/pkg/gen"
)

// InsertRequest carries a driver-owned [InsertMethod], a worker count, and a
// [gen.BatchSource] whose prepared partitions fill reusable typed batches.
// Workloads hand their source straight to the driver, so generation avoids
// intermediate protobuf values.
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
		return ErrNilInsertRequest
	}

	if req.Source == nil {
		return ErrNilInsertSource
	}

	if req.Source.Schema().Columns() == 0 {
		return gen.ErrEmptySchema
	}

	return nil
}
