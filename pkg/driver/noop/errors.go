package noop

import "github.com/stroppy-io/stroppy/v6/pkg/driver"

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	return driver.DefaultErrorFacts(err)
}
