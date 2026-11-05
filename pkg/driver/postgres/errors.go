package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func statementContextError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	ctxErr := ctx.Err()
	if ctxErr == nil || errors.Is(err, ctxErr) {
		return err
	}

	return errors.Join(ctxErr, err)
}

func (*Driver) ClassifyError(err error) driver.ErrorFacts {
	if errors.Is(err, context.Canceled) {
		return driver.ErrorFacts{Kind: driver.ErrorKindCanceled}
	}

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505":
			return driver.ErrorFacts{Kind: driver.ErrorKindUniqueViolation}
		case "23000", "23001", "23502", "23503", "23514", "23P01":
			return driver.ErrorFacts{Kind: driver.ErrorKindConstraintViolation}
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
