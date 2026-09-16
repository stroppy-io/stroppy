package ydb

import (
	"errors"

	ydbretry "github.com/ydb-platform/ydb-go-sdk/v3/retry"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	if errors.Is(err, ErrUnsupportedInsertMethod) || errors.Is(err, ErrUnsupportedType) {
		return driver.ErrorFacts{Kind: driver.ErrorKindUnsupported}
	}

	// The SDK can join a retryable server error with cancellation of its query
	// stream. Preserve the SDK retry decision; the caller context is checked by
	// the retry loop before another attempt or while waiting for backoff.
	mode := ydbretry.Check(err)
	switch {
	case mode.MustRetry(false):
		return driver.ErrorFacts{Kind: driver.ErrorKindTransient, Backoff: mode.MustBackoff()}
	case mode.MustRetry(true):
		return driver.ErrorFacts{
			Kind:                driver.ErrorKindTransient,
			Backoff:             mode.MustBackoff(),
			RequiresIdempotency: true,
		}
	default:
		return driver.DefaultErrorFacts(err)
	}
}
