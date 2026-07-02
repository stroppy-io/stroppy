package driver

// Isolation is a transaction isolation level, ported from v5's TxIsolationLevel
// (internal/static/helpers.ts, pkg/driver/postgres/tx.go). Besides the four
// SQL-standard levels it carries three engine-shaped modes:
//
//   - DBDefault leaves the level unset so the server default applies. It is the
//     zero value, so an unspecified Isolation never silently selects a weaker
//     level than the operator's database default.
//   - ConnectionOnly mirrors v5's CONNECTION_ONLY: no BEGIN is issued;
//     statements run directly on the pinned connection at its current session
//     isolation, and Commit/Rollback are no-ops. Used by engines
//     (picodata-class) whose Begin is unsupported.
//   - None runs statements with no transaction at all — calls pass straight
//     through to the connection, Commit/Rollback are no-ops.
//
// For a driver with one pinned connection per VU, ConnectionOnly and None
// behave identically (both pass through to that connection without a BEGIN);
// the two values are kept distinct to preserve v5's semantics and intent.
// (ConnectionOnly is spelled out rather than "Conn" to avoid colliding with
// the Conn interface.)
type Isolation uint8

// Isolation levels. DBDefault is the zero value.
const (
	DBDefault Isolation = iota
	ReadUncommitted
	ReadCommitted
	RepeatableRead
	Serializable
	ConnectionOnly
	None
)

// String returns the v5 isolation name.
func (i Isolation) String() string {
	switch i {
	case DBDefault:
		return "db_default"
	case ReadUncommitted:
		return "read_uncommitted"
	case ReadCommitted:
		return "read_committed"
	case RepeatableRead:
		return "repeatable_read"
	case Serializable:
		return "serializable"
	case ConnectionOnly:
		return "conn"
	case None:
		return "none"
	default:
		return "unknown"
	}
}
