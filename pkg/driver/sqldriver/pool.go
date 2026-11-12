package sqldriver

import (
	"database/sql"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func PoolSnapshot(db *sql.DB, scope string) driver.PoolSnapshot {
	stats := db.Stats()

	return driver.PoolSnapshot{
		Backend: "database/sql", Scope: scope, Supported: true,
		MaxConnections: int64(stats.MaxOpenConnections),
		Open:           int64(stats.OpenConnections), InUse: int64(stats.InUse), Idle: int64(stats.Idle),
		WaitCount: stats.WaitCount, WaitDuration: stats.WaitDuration,
	}
}
