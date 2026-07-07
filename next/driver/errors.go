package driver

import "errors"

// ErrNoRows is reported by a Row (and its scans) when the query returned no
// row. Drivers surface their own no-rows sentinel through this value so callers
// stay driver-agnostic.
var ErrNoRows = errors.New("driver: no rows in result set")

// Action is the resolved run-level outcome for an error, as classified by the
// backend that produced it (see [Driver.Classify]). It is the single input the
// query/tx wrapper (D9's bench.Transaction) and the executor feed into their
// retry/lifecycle decisions — there is no separate global matcher.
//
// The steady-state hot path returns nil and never consults Classify, so the
// enum only exists on the rare failure branch.
type Action int

const (
	// Retry marks a transient, serialization-class failure the wrapper should
	// replay (pg 40001/40P01, ydb transient, …). Only the dbdrv knows which
	// backend errors are transient, hence Classify lives on the driver.
	Retry Action = iota
	// Continue swallows the error into telemetry and proceeds. The default for
	// a plain non-retryable error and for a retry budget exhausted.
	Continue
	// Fail completes the current step, then halts the run with this step
	// marked Failed. Motivated by validation: every assertion still runs.
	Fail
	// Abort halts the run immediately — cancel in-flight work, drain Close,
	// exit non-zero. For connection-lost and SDK-system faults.
	Abort
)

// String renders the Action name.
func (a Action) String() string {
	switch a {
	case Retry:
		return "Retry"
	case Continue:
		return "Continue"
	case Fail:
		return "Fail"
	case Abort:
		return "Abort"
	default:
		return "Unknown"
	}
}

// sqlStater is implemented by driver errors that carry a SQLSTATE code
// (pgconn.PgError does, via SQLState). Matching on this interface lets a
// concrete driver classify errors without the base package importing any SQL
// driver. Kept unexported: only pg's Classify reads it today; a future backend
// implements Classify however it likes.
type sqlStater interface {
	SQLState() string
}

// SQLState unwraps err and reports its SQLSTATE code, if any. It is the shared
// SQLSTATE-extraction helper concrete drivers (pg today) build their Classify
// on, so the unwrap-once logic is not duplicated per backend.
func SQLState(err error) (string, bool) {
	var s sqlStater
	if errors.As(err, &s) {
		return s.SQLState(), true
	}
	return "", false
}
