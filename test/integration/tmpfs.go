//go:build integration

// Package integration provides end-to-end helpers for the PostgreSQL and
// MySQL services managed by test/compose.tmpfs.yml.
package integration

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultTmpfsURL = "postgres://postgres:postgres@localhost:5434/stroppy"
	envTmpfsURL     = "STROPPY_TMPFS_URL"
)

// NewTmpfsPG connects to the baseline PostgreSQL service and returns a scoped pool.
// STROPPY_TMPFS_URL overrides the local compose URL.
func NewTmpfsPG(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv(envTmpfsURL)
	if url == "" {
		url = defaultTmpfsURL
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New(%q): %v", url, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("pool.Ping: %v (is `make tmpfs-up` running?)", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// ResetSchema drops and recreates the public schema so each test starts clean.
func ResetSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	const stmt = `DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO postgres;`
	if _, err := pool.Exec(context.Background(), stmt); err != nil {
		t.Fatalf("ResetSchema: %v", err)
	}
}

// CountRows returns the number of rows in the given table.
func CountRows(t *testing.T, pool *pgxpool.Pool, table string) int64 {
	t.Helper()

	var n int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if err := pool.QueryRow(context.Background(), query).Scan(&n); err != nil {
		t.Fatalf("CountRows(%s): %v", table, err)
	}
	return n
}

// AssertTableEquals runs the given SELECT and compares the returned rows
// against want in order. Column names are taken from the result field
// descriptions; values are compared with reflect.DeepEqual.
func AssertTableEquals(t *testing.T, pool *pgxpool.Pool, query string, want []map[string]any) {
	t.Helper()

	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("AssertTableEquals: query %q: %v", query, err)
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = string(f.Name)
	}

	var got []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			t.Fatalf("AssertTableEquals: rows.Values: %v", err)
		}
		row := make(map[string]any, len(cols))
		for i, name := range cols {
			row[name] = values[i]
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("AssertTableEquals: rows.Err: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("AssertTableEquals: row count mismatch\n  query: %s\n  got:   %d rows (%v)\n  want:  %d rows (%v)",
			query, len(got), got, len(want), want)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("AssertTableEquals: row %d mismatch\n  query: %s\n  got:   %#v\n  want:  %#v",
				i, query, got[i], want[i])
		}
	}
}
