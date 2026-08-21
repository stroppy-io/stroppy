package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "40001":
			return driver.ErrorFacts{Kind: driver.ErrorKindSerialization}
		case "40P01":
			return driver.ErrorFacts{Kind: driver.ErrorKindDeadlock}
		case "55P03":
			return driver.ErrorFacts{Kind: driver.ErrorKindLockTimeout}
		case "57014":
			return driver.ErrorFacts{Kind: driver.ErrorKindTimeout}
		}
	}

	return driver.DefaultErrorFacts(err)
}
