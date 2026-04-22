package loader

import "errors"

// ErrNilInserter is returned by New when the supplied Inserter is nil.
// A Loader cannot admit work without a driver adapter to dispatch it to.
var ErrNilInserter = errors.New("loader: nil Inserter")

// ErrNilSpec is returned by Insert / InsertConcurrent when any InsertSpec
// pointer is nil. The spec carries the table, source, and parallelism
// hint; the Loader cannot schedule work without it.
var ErrNilSpec = errors.New("loader: nil InsertSpec")

// ErrZeroCap is returned by New when totalWorkerCap is not strictly
// positive. The global cap is a hard budget on concurrent workers; zero
// or negative values would deadlock Acquire or permit unbounded fan-out.
var ErrZeroCap = errors.New("loader: totalWorkerCap must be > 0")
