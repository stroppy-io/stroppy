package mysql

import (
	"github.com/stroppy-io/stroppy/next/driver"
)

// isoLevel reports whether iso starts a real transaction and the mysql name of
// its isolation level. An empty level (DBDefault) leaves the server default in
// place; ConnectionOnly and None pass through (no BEGIN — statements run
// directly on the connection).
//
// Manual transactions (rather than database/sql's *sql.Tx) are deliberate: a
// *sql.Stmt prepared on the connection is reused across transactions this way,
// which *sql.Tx forbids (a conn-prepared statement cannot execute inside a
// *sql.Tx). mysql associates the transaction with the session/connection, so a
// conn-bound prepared statement executes inside the manually begun transaction.
// go-sql-driver/mysql rejects multi-statement Exec by default, so the caller
// issues SET (when a level is set) and START TRANSACTION as separate Execs.
func isoLevel(iso driver.Isolation) (level string, realTx bool) {
	switch iso {
	case driver.ReadUncommitted:
		return "READ UNCOMMITTED", true
	case driver.ReadCommitted:
		return "READ COMMITTED", true
	case driver.RepeatableRead:
		return "REPEATABLE READ", true
	case driver.Serializable:
		return "SERIALIZABLE", true
	case driver.DBDefault:
		return "", true
	default: // ConnectionOnly, None
		return "", false
	}
}
