package csv

import (
	"errors"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	if errors.Is(err, ErrCsvDriverNoQuery) || errors.Is(err, ErrUnsupportedInsertMethod) {
		return driver.ErrorFacts{Kind: driver.ErrorKindUnsupported}
	}

	return driver.DefaultErrorFacts(err)
}
