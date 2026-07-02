// Package driver defines the engine-facing database interfaces — Driver, Conn,
// Tx, Stmt, Row and Rows — that concrete drivers such as driver/pg and
// driver/noop implement (RFC 0001 §10).
//
// # No metrics inside the driver
//
// This package and its implementations deliberately do NOT import next/bench or
// next/metrics and record nothing themselves. In v5 every query/tx/insert was
// timed inside the driver and pushed through k6's sample stream; that coupling
// is removed here. The caller (the executor in next/bench) times each call and
// records into its own per-VU metric shard. The driver's only job is to move
// bytes to and from the database. It may import sqlfile (for prepared-query
// text) and mem (for the columnar insert buffer), nothing higher.
//
// # Two bind paths
//
// The query methods come in two flavours:
//
//   - The variadic convenience path — Exec(ctx, stmt, args...) — allocates the
//     variadic slice on every call. Use it for cold steps (DDL, setup, one-shot
//     validation) where an allocation per call is irrelevant.
//   - The reusable-buffer path — Stmt.Bind returns an *Args whose typed setters
//     write into storage reused across iterations; ExecWithArgs/QueryWithArgs/
//     QueryRowWithArgs take that *Args. This is the hot path: no per-call
//     variadic slice. See Args for the per-value boxing cost a real SQL driver
//     still pays when it materialises the buffer for its wire protocol.
package driver
