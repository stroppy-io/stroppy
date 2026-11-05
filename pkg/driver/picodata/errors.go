package picodata

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	if errors.Is(err, ErrNativeUnsupported) ||
		errors.Is(err, ErrTransactionsUnsupported) ||
		errors.Is(err, ErrUnsupportedType) {
		return driver.ErrorFacts{Kind: driver.ErrorKindUnsupported}
	}

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case pgErr.Code == "23505" || strings.Contains(pgErr.Message, "TupleFound: Duplicate key exists in unique index"):
			return driver.ErrorFacts{Kind: driver.ErrorKindUniqueViolation}
		case pgErr.Code == "23502", pgErr.Code == "23503", pgErr.Code == "23514":
			return driver.ErrorFacts{Kind: driver.ErrorKindConstraintViolation}
		}
	}

	return driver.DefaultErrorFacts(err)
}
