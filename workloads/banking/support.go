package banking

import (
	"context"
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// sqlSection runs every query of one SQL section in order, for setup and
// once-only steps.
type sqlSection struct {
	sql  *bench.SQL
	name string
}

func (s sqlSection) Run(ctx context.Context, b *bench.Bench) error {
	for _, q := range s.sql.Section(s.name) {
		if err := b.Exec(ctx, q, nil); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}

	return nil
}

// sqlFileFor names the asset file for a driver: one SQL file per dialect, each
// with the same sections and named queries.
func sqlFileFor(dt bench.DriverTypeName) string {
	switch dt {
	case bench.DriverMySQL, bench.DriverTiDB:
		return "mysql.sql"
	case bench.DriverPicodata:
		return "pico.sql"
	case bench.DriverYDB:
		return "ydb.sql"
	default:
		// postgres, and the engines that speak its protocol. Measured: TiDB reached
		// this default until it was given its own case, which loaded the postgres
		// asset over the mysql wire.
		return "pg.sql"
	}
}

// resolveIsolation mirrors the per-driver defaults the other transactional
// workloads use: picodata has no working Begin, ydb is serializable only, and
// everything else takes read committed.
func resolveIsolation(dt bench.DriverTypeName, override bench.TxIsolationName) bench.TxIsolationName {
	if override != "" {
		return override
	}

	switch dt {
	case bench.DriverPicodata:
		return bench.IsoNone
	case bench.DriverYDB:
		return bench.IsoSerializable
	default:
		return bench.IsoReadCommitted
	}
}
