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
