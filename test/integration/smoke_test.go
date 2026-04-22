//go:build integration

package integration

import (
	"context"
	"testing"
)

// TestTmpfsSmoke verifies that the tmpfs Postgres harness is reachable and
// that the helpers round-trip a trivial table end-to-end.
func TestTmpfsSmoke(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE test_table (id int, name text)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO test_table (id, name) VALUES ($1, $2)`, 1, "hello"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if got := CountRows(t, pool, "test_table"); got != 1 {
		t.Fatalf("CountRows = %d, want 1", got)
	}

	AssertTableEquals(t, pool, `SELECT id, name FROM test_table ORDER BY id`, []map[string]any{
		{"id": int32(1), "name": "hello"},
	})
}
