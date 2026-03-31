package sqldriver

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// ApplySQLConfig applies SQL pool configuration to a *sql.DB.
// Returns an error if any duration string is malformed.
func ApplySQLConfig(db *sql.DB, sqlCfg *stroppy.DriverConfig_SqlConfig) error {
	if sqlCfg == nil {
		return nil
	}

	if sqlCfg.MaxOpenConns != nil {
		db.SetMaxOpenConns(int(sqlCfg.GetMaxOpenConns()))
	}

	if sqlCfg.MaxIdleConns != nil {
		db.SetMaxIdleConns(int(sqlCfg.GetMaxIdleConns()))
	}

	if sqlCfg.ConnMaxLifetime != nil {
		d, err := time.ParseDuration(sqlCfg.GetConnMaxLifetime())
		if err != nil {
			return fmt.Errorf("invalid conn_max_lifetime %q: %w", sqlCfg.GetConnMaxLifetime(), err)
		}

		db.SetConnMaxLifetime(d)
	}

	if sqlCfg.ConnMaxIdleTime != nil {
		d, err := time.ParseDuration(sqlCfg.GetConnMaxIdleTime())
		if err != nil {
			return fmt.Errorf("invalid conn_max_idle_time %q: %w", sqlCfg.GetConnMaxIdleTime(), err)
		}

		db.SetConnMaxIdleTime(d)
	}

	return nil
}

// DBPinger adapts *sql.DB to the Ping interface expected by WaitForDB.
type DBPinger struct {
	DB *sql.DB
}

func (p *DBPinger) Ping(ctx context.Context) error {
	return p.DB.PingContext(ctx)
}
