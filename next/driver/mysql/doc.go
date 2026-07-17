// Package mysql is the database/sql + go-sql-driver/mysql backed [driver.Driver].
//
// It proves the N+M universal-driver invariant: a second concrete driver over
// the same engine-facing interfaces, where pg is the pgx-backed first. mysql
// has no COPY equivalent, so [driver.InsertNative] resolves to multi-row bulk
// here ([driver.InsertPlainBulk]); [driver.InsertColumnar] is unsupported and
// reported explicitly. mysql's "?" placeholder is per-occurrence, so a prepared
// [driver.Stmt] expands a named [driver.Args] to occurrence order via
// [sqlfile.Query.Occurrences] at materialise time.
//
// Like the other drivers, this package records no metrics — the bench layer
// times each call.
package mysql
