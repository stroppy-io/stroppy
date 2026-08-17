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

	facts := driver.DefaultErrorFacts(err)
	if facts.Kind != driver.ErrorKindUnknown {
		return facts
	}

	mode := ydbretry.Check(err)
	switch {
	case mode.MustRetry(false):
		return driver.ErrorFacts{Kind: driver.ErrorKindTransient, Backoff: mode.MustBackoff()}
	case mode.MustRetry(true):
		return driver.ErrorFacts{Kind: driver.ErrorKindTransientIfIdempotent, Backoff: mode.MustBackoff()}
	default:
		return facts
	}
}
