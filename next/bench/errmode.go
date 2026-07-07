package bench

import "fmt"

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

// ErrorMode decides what an executor does with an Iter error that the driver
// classifier (D9's tx wrapper) did not resolve and that is not an explicit
// [Fail]/[Abort] root error. It is the Go port of v5's ErrorModeName
// (silent|log|throw|fail|abort); see the package doc for the full mapping and
// the rationale for merging throw into Fail.
//
// The constants carry a Mode prefix so the canonical error constructors
// [Fail] and [Abort] keep the bare names the D10 decision specifies.
//
// Per-error override: a Handler may emit [Fail] or [Abort] to force the
// run-level outcome regardless of the step's configured ErrorMode — validation
// uses Fail so every assertion still runs; connection-lost uses Abort to halt
// immediately.
type ErrorMode int

const (
	// ModeLog counts the error, logs it to stderr, and keeps running. Default
	// (the zero value), matching v5's default of "log".
	ModeLog ErrorMode = iota
	// ModeSilent counts the error and keeps running, without logging. (v5 "silent")
	ModeSilent
	// ModeFail counts and logs the error and keeps running, but the executor's
	// Run returns the first such error as an aggregate. (v5 "throw" and "fail")
	ModeFail
	// ModeAbort counts and logs the error, cancels the executor context so
	// in-flight Iters finish and every VU's Close runs, and Run returns promptly
	// with the error. (v5 "abort")
	ModeAbort
)

// String renders the ErrorMode name.
func (m ErrorMode) String() string {
	switch m {
	case ModeLog:
		return "log"
	case ModeSilent:
		return "silent"
	case ModeFail:
		return "fail"
	case ModeAbort:
		return "abort"
	default:
		return "unknown"
	}
}
