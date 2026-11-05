package mysql

import (
	"errors"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

const (
	errDuplicateEntry         = 1062
	errNotNull                = 1048
	errForeignKeyMissing      = 1216
	errForeignKeyReferenced   = 1217
	errRowReferenced          = 1451
	errNoReferencedRow        = 1452
	errCheckConstraint        = 3819
	errMariaDBCheckConstraint = 4025
	errRecordChanged          = 1020 // ER_CHECKREAD: row changed since this transaction's snapshot
	errLockDeadlock           = 1213 // ER_LOCK_DEADLOCK
	errLockWaitTimeout        = 1205 // ER_LOCK_WAIT_TIMEOUT
	errQueryTimeout           = 3024 // ER_QUERY_TIMEOUT (MAX_EXECUTION_TIME) — aborted with max_execution_time exceeded
)

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	if myErr, ok := errors.AsType[*gomysql.MySQLError](err); ok {
		switch myErr.Number {
		case errDuplicateEntry:
			return driver.ErrorFacts{Kind: driver.ErrorKindUniqueViolation}
		case errNotNull, errForeignKeyMissing, errForeignKeyReferenced, errRowReferenced,
			errNoReferencedRow, errCheckConstraint, errMariaDBCheckConstraint:
			return driver.ErrorFacts{Kind: driver.ErrorKindConstraintViolation}
		case errRecordChanged:
			return driver.ErrorFacts{Kind: driver.ErrorKindSerialization}
		case errLockDeadlock:
			return driver.ErrorFacts{Kind: driver.ErrorKindDeadlock}
		case errLockWaitTimeout:
			return driver.ErrorFacts{Kind: driver.ErrorKindLockTimeout}
		case errQueryTimeout:
			return driver.ErrorFacts{Kind: driver.ErrorKindTimeout}
		}
	}

	return driver.DefaultErrorFacts(err)
}
