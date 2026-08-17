package picodata

import (
	"errors"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	if errors.Is(err, ErrNativeUnsupported) ||
		errors.Is(err, ErrTransactionsUnsupported) ||
		errors.Is(err, ErrUnsupportedType) {
		return driver.ErrorFacts{Kind: driver.ErrorKindUnsupported}
	}

	return driver.DefaultErrorFacts(err)
}
