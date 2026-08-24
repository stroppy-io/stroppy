package driver

import (
	"context"
	"errors"
)

// ErrorKind describes a database-independent property of a driver error.
type ErrorKind string

const (
	ErrorKindUnknown       ErrorKind = "unknown"
	ErrorKindSerialization ErrorKind = "serialization"
	ErrorKindDeadlock      ErrorKind = "deadlock"
	ErrorKindLockTimeout   ErrorKind = "lock_timeout"
	ErrorKindTransient     ErrorKind = "transient"
	ErrorKindUnsupported   ErrorKind = "unsupported"
	ErrorKindCanceled      ErrorKind = "canceled"
	ErrorKindTimeout       ErrorKind = "timeout"
)

// ErrorFacts are backend-neutral properties used by workload error policy.
type ErrorFacts struct {
	Kind                ErrorKind
	Backoff             bool
	RequiresIdempotency bool
}

// DefaultErrorFacts classifies errors shared by every driver. Unknown errors
// remain explicit so policy defaults fail closed instead of retrying them.
func DefaultErrorFacts(err error) ErrorFacts {
	switch {
	case errors.Is(err, context.Canceled):
		return ErrorFacts{Kind: ErrorKindCanceled}
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorFacts{Kind: ErrorKindTimeout}
	default:
		return ErrorFacts{Kind: ErrorKindUnknown}
	}
}

// JoinErrors combines distinct error causes while preserving errors.Is behavior.
func JoinErrors(errs ...error) error {
	unique := make([]error, 0, len(errs))
	for _, err := range errs {
		appendDistinctError(&unique, err)
	}

	return errors.Join(unique...)
}

func appendDistinctError(unique *[]error, err error) {
	if err == nil {
		return
	}

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			appendDistinctError(unique, cause)
		}

		return
	}

	for _, candidate := range *unique {
		if sameErrorCause(candidate, err) {
			return
		}
	}

	*unique = append(*unique, err)
}

func sameErrorCause(left, right error) bool {
	return errors.Is(left, right) || errors.Is(right, left)
}
