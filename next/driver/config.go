package driver

import "time"

// Config configures a Driver. It is intentionally minimal for the PoC: a
// connection URL plus a few conn/pool knobs. Pinned-conn drivers (the default,
// one dedicated connection per VU) use URL and ConnectTimeout and ignore the
// MinConns/MaxConns pool bounds, which are reserved for a later pooled mode.
type Config struct {
	// URL is the driver-specific connection string (e.g. a libpq/pgx DSN).
	URL string
	// MinConns, MaxConns bound a connection pool if the driver runs in pooled
	// mode. Pinned-conn drivers ignore them.
	MinConns int32
	MaxConns int32
	// ConnectTimeout caps a single Connect. Zero means the driver default.
	ConnectTimeout time.Duration
}
