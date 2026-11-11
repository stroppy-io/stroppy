package driver

import "time"

// PoolStatistics is optional because some SDKs hide their pools or use no pool.
type PoolStatistics interface {
	PoolStats() PoolSnapshot
}

type PoolSnapshot struct {
	Backend string
	Scope   string
	Reason  string
	// Unsupported pools must not look like supported pools with no contention.
	Supported bool
	// Zero means unlimited; a negative limit means unknown.
	MaxConnections int64
	Open           int64
	InUse          int64
	Idle           int64
	WaitCount      int64
	WaitDuration   time.Duration
	Canceled       int64
}
