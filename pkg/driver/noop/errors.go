package noop

import "github.com/stroppy-io/stroppy/pkg/driver"

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	return driver.DefaultErrorFacts(err)
}
