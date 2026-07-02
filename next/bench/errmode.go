package bench

import (
	"fmt"
	"time"
)

// vuError wraps an Init/Close failure with the VU index and lifecycle stage, so
// an aggregate Run error names where it came from.
type vuError struct {
	stage string
	vu    int
	err   error
}

func (e *vuError) Error() string {
	return fmt.Sprintf("vu %d %s: %v", e.vu, e.stage, e.err)
}

func (e *vuError) Unwrap() error { return e.err }

// ErrorMode decides what an executor does with an Iter error that a
// [RetryPolicy] did not resolve. It is the Go port of v5's ErrorModeName
// (silent|log|throw|fail|abort); see the package doc for the full mapping and
// the rationale for merging throw into Fail.
type ErrorMode int

const (
	// Log counts the error, logs it to stderr, and keeps running. Default
	// (the zero value), matching v5's default of "log".
	Log ErrorMode = iota
	// Silent counts the error and keeps running, without logging. (v5 "silent")
	Silent
	// Fail counts and logs the error and keeps running, but the executor's Run
	// returns the first such error as an aggregate. (v5 "throw" and "fail")
	Fail
	// Abort counts and logs the error, cancels the executor context so in-flight
	// Iters finish and every VU's Close runs, and Run returns promptly with the
	// error. (v5 "abort")
	Abort
)

// String renders the ErrorMode name.
func (m ErrorMode) String() string {
	switch m {
	case Log:
		return "log"
	case Silent:
		return "silent"
	case Fail:
		return "fail"
	case Abort:
		return "abort"
	default:
		return "unknown"
	}
}

// RetryPolicy bounds retry of a single Iter. The zero value is a single attempt
// with no retry. It is applied inside the executor loop, before the [ErrorMode]
// classification sees the error, so a retried-then-succeeded iteration surfaces
// no error at all.
//
// It is structurally identical to dag.RetryPolicy; the wiring milestone may
// unify them once an executor is a dag node.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts including the first. Values
	// below 1 mean a single attempt.
	MaxAttempts int
	// Backoff returns the delay before the next attempt given the 1-based
	// number of the attempt that just failed. Nil means no delay. Kept as a
	// function (not a fixed duration) so serialization retries can be immediate
	// while transient-network retries back off — matching v5's retry helpers.
	Backoff func(attempt int) time.Duration
	// Retryable classifies an error as worth retrying. Nil means never retry,
	// regardless of MaxAttempts. (M5 wires the SQLSTATE 40001 / deadlock
	// classifier here, porting v5's isSerializationError.)
	Retryable func(err error) bool
}
