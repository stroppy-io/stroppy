package tpcc

import (
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

// A protocol error carries a code and an unformatted message. Error() adds
// backend-specific prefixes, so matching its prefix loses expected rollbacks.
// A joined error may wrap the same deduplicated row error. Additional causes
// are not accepted: another failure may describe an unknown rollback outcome.
func isRollbackSentinel(err error) bool {
	if joined, ok := errors.AsType[interface {
		error
		Unwrap() []error
	}](err); ok {
		causes := joined.Unwrap()

		return len(causes) == 1 && isRollbackSentinel(causes[0])
	}

	if errors.Is(err, errRollbackSentinel) {
		return true
	}

	if typed, ok := errors.AsType[*pgconn.PgError](err); ok {
		return typed.Code == "P0001" && typed.Message == errRollbackSentinel.Error()
	}

	if typed, ok := errors.AsType[*mysql.MySQLError](err); ok {
		return typed.Number == 1644 && typed.SQLState == [5]byte{'4', '5', '0', '0', '0'} &&
			typed.Message == errRollbackSentinel.Error()
	}

	return false
}
