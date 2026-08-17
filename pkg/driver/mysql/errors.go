package mysql

import (
	"errors"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

const (
	errLockDeadlock    = 1213 // ER_LOCK_DEADLOCK
	errLockWaitTimeout = 1205 // ER_LOCK_WAIT_TIMEOUT
)

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	if myErr, ok := errors.AsType[*gomysql.MySQLError](err); ok {
		switch myErr.Number {
		case errLockDeadlock:
			return driver.ErrorFacts{Kind: driver.ErrorKindDeadlock}
		case errLockWaitTimeout:
			return driver.ErrorFacts{Kind: driver.ErrorKindLockTimeout}
		}
	}

	return driver.DefaultErrorFacts(err)
}
