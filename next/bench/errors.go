package bench

import (
	"errors"
	"fmt"
)

// Root errors — Fail and Abort — let a Handler author pin the run-level outcome
// of an emitted error directly, overriding the step's [ErrorMode]. They are the
// D10 realization of the k6 fail/abort distinction at step granularity:
//
//   - Fail completes the current step, then halts the run with this step marked
//     Failed. Validation emits Fail-rooted errors so every assertion still
//     executes, the step is marked failed, and the test halts after — not
//     mid-assertion.
//   - Abort halts the run immediately: cancel in-flight work, drain every VU's
//     Close, exit non-zero. Connection-lost and SDK-system faults use Abort.
//
// The classifier Action from driver.Classify (Retry/Continue/Fail/Abort) is the
// same vocabulary consumed by D9's bench.Transaction helper; Fail/Abort here are
// the author-facing constructors for the two non-default root kinds. A plain
// fmt.Errorf return is Continue (counted in metrics, run proceeds).
//
// Native Go errors only — no panics, no throw (D10 principle). The rooted error
// implements error and unwraps to its cause, so errors.Is/As and fmt error
// formatting compose normally.

// rootKind tags an error with the author's intended run-level outcome.
type rootKind int

const (
	kindFail  rootKind = iota + 1 // non-zero so the zero rootKind means "not rooted"
	kindAbort
)

// rootedError wraps an error with a rootKind. The executor's onError inspects
// the kind via rootAction; other callers unwrap to read the cause.
type rootedError struct {
	kind rootKind
	err  error
}

func (e *rootedError) Error() string {
	switch e.kind {
	case kindFail:
		return fmt.Sprintf("fail: %v", e.err)
	case kindAbort:
		return fmt.Sprintf("abort: %v", e.err)
	default:
		return e.err.Error()
	}
}

func (e *rootedError) Unwrap() error { return e.err }

// Fail wraps err to signal a validation-style failure: let the current step run
// to completion, then halt the run with this step marked Failed (no subsequent
// steps execute). nil err is accepted for a signal-only Fail and renders as the
// bare kind.
func Fail(err error) error {
	return &rootedError{kind: kindFail, err: err}
}

// Abort wraps err to signal an immediate run halt: cancel in-flight work, run
// every VU's Close, and exit non-zero. nil err is accepted for a signal-only
// Abort.
func Abort(err error) error {
	return &rootedError{kind: kindAbort, err: err}
}

// rootAction classifies err's root kind for the executor. A non-rooted error
// (the common case: a plain fmt.Errorf from a Handler) returns ok=false so the
// caller falls back to its configured [ErrorMode].
func rootAction(err error) (rootKind, bool) {
	var r *rootedError
	if errors.As(err, &r) {
		return r.kind, true
	}
	return 0, false
}

// IsFail reports whether err is Fail-rooted. It is the query form for callers
// that only need the fail/abort distinction (e.g. a future step translator).
func IsFail(err error) bool {
	k, ok := rootAction(err)
	return ok && k == kindFail
}

// IsAbort reports whether err is Abort-rooted.
func IsAbort(err error) bool {
	k, ok := rootAction(err)
	return ok && k == kindAbort
}
