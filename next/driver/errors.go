package driver

import "errors"

// ErrNoRows is reported by a Row (and its scans) when the query returned no
// row. Drivers surface their own no-rows sentinel through this value so callers
// stay driver-agnostic.
var ErrNoRows = errors.New("driver: no rows in result set")

// sqlStater is implemented by driver errors that carry a SQLSTATE code
// (pgconn.PgError does, via SQLState). Matching on this interface lets the base
// package classify errors without importing any concrete SQL driver.
type sqlStater interface {
	SQLState() string
}

// IsRetryable reports whether err is a transient serialization failure that a
// transaction may retry, porting v5's isSerializationError (helpers.ts): a
// serialization failure (SQLSTATE 40001) or a deadlock (SQLSTATE 40P01). The
// error is unwrapped (errors.As) to find a SQLSTATE-bearing cause, so wrapping
// with %w does not hide it. Any other error — including application rollbacks
// raised with RAISE EXCEPTION (e.g. tpcc's item-not-found sentinel, SQLSTATE
// P0001) — is not retryable.
func IsRetryable(err error) bool {
	var s sqlStater
	if errors.As(err, &s) {
		switch s.SQLState() {
		case "40001", "40P01":
			return true
		}
	}

	return false
}
