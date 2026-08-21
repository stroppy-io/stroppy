//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	_ "github.com/stroppy-io/stroppy/pkg/driver/mysql"
	_ "github.com/stroppy-io/stroppy/pkg/driver/postgres"
)

// dispatchQueryTimeout builds a stroppy driver with a per-statement deadline.
func dispatchQueryTimeout(
	t *testing.T,
	typ stroppy.DriverConfig_DriverType,
	url string,
	timeout time.Duration,
) driver.Driver {
	t.Helper()

	drv, err := driver.Dispatch(context.Background(), driver.Options{
		Config:       &stroppy.DriverConfig{DriverType: typ, Url: url},
		Logger:       zap.NewExample(),
		QueryTimeout: timeout,
	})
	if err != nil {
		t.Fatalf("driver.Dispatch(%s): %v", typ, err)
	}

	t.Cleanup(func() { _ = drv.Teardown(context.Background()) })

	return drv
}

func runQueryToCompletion(drv driver.Driver, sql string) error {
	res, err := drv.RunQuery(context.Background(), sql, nil)
	if err != nil {
		return err
	}
	defer res.Rows.Close()

	res.Rows.ReadAll(0)

	return res.Rows.Err()
}

// assertReusable drives a fast query on the same driver after a timeout to
// prove the pooled connection is still serviceable.
func assertReusable(t *testing.T, drv driver.Driver) {
	t.Helper()

	res, err := drv.RunQuery(context.Background(), "SELECT 1", nil)
	if err != nil {
		t.Fatalf("reuse query = %v, want nil", err)
	}
	defer res.Rows.Close()

	if !res.Rows.Next() {
		t.Fatal("reuse query returned no rows")
	}

	if err := res.Rows.Err(); err != nil {
		t.Fatalf("reuse query rows: %v", err)
	}
}

func assertMySQLClientTimeout(
	t *testing.T,
	drv driver.Driver,
	sql string,
	timeout time.Duration,
) {
	t.Helper()

	start := time.Now()
	err := runQueryToCompletion(drv, sql)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("query %q returned nil error, want client timeout", sql)
	}
	if facts := drv.ClassifyError(err); facts.Kind != driver.ErrorKindTimeout {
		t.Fatalf("ClassifyError = %q, want %q (err=%v)", facts.Kind, driver.ErrorKindTimeout, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("query %q error = %v, want context.DeadlineExceeded", sql, err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("query %q error = %v was classified as canceled", sql, err)
	}
	if limit := timeout + 500*time.Millisecond; elapsed > limit {
		t.Fatalf("query %q timed out after %v, want client timeout within %v", sql, elapsed, limit)
	}

	assertReusable(t, drv)
}

// TestQueryTimeoutPostgres blocks pg_sleep past the deadline and verifies the
// statement is classified as a timeout rather than a parent cancel, and that
// the pooled connection is reusable afterward.
func TestQueryTimeoutPostgres(t *testing.T) {
	skipIfRequested(t)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	drv := dispatchQueryTimeout(t, stroppy.DriverConfig_DRIVER_TYPE_POSTGRES, url, 150*time.Millisecond)

	start := time.Now()
	err := runQueryToCompletion(drv, "SELECT pg_sleep(10)")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("pg_sleep returned nil error, want timeout")
	}
	if facts := drv.ClassifyError(err); facts.Kind != driver.ErrorKindTimeout {
		t.Fatalf("ClassifyError = %q, want %q (err=%v)", facts.Kind, driver.ErrorKindTimeout, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "57014" {
			t.Fatalf("pg timeout err = %v, want deadline or SQLSTATE 57014", err)
		}
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("pg timeout err = %v was classified as canceled", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("pg_sleep(10) ran %v, deadline did not fire", elapsed)
	}

	assertReusable(t, drv)
}

// TestQueryTimeoutMySQL runs a large metadata join past the deadline and verifies
// the server-side MAX_EXECUTION_TIME hint (error 3024) fires ahead of the padded
// client deadline, so the connection is not discarded and stays reusable.
func TestQueryTimeoutMySQL(t *testing.T) {
	skipIfRequested(t)

	url := envOr(envMySQLAllURL, defaultMySQLAllURL)
	drv := dispatchQueryTimeout(t, stroppy.DriverConfig_DRIVER_TYPE_MYSQL, url, 150*time.Millisecond)

	const query = "SELECT COUNT(*) FROM information_schema.columns a " +
		"CROSS JOIN information_schema.columns b CROSS JOIN information_schema.columns c"

	start := time.Now()
	err := runQueryToCompletion(drv, query)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("large metadata join returned nil error, want timeout")
	}
	if facts := drv.ClassifyError(err); facts.Kind != driver.ErrorKindTimeout {
		t.Fatalf("ClassifyError = %q, want %q (err=%v)", facts.Kind, driver.ErrorKindTimeout, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("mysql timeout err = %v reached the client deadline, want server-side 3024", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("mysql timeout err = %v was classified as canceled", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("large metadata join ran %v, deadline did not fire", elapsed)
	}

	assertReusable(t, drv)
}

func TestQueryTimeoutMySQLUnhintedStatements(t *testing.T) {
	skipIfRequested(t)

	const timeout = 150 * time.Millisecond

	url := envOr(envMySQLAllURL, defaultMySQLAllURL)
	drv := dispatchQueryTimeout(t, stroppy.DriverConfig_DRIVER_TYPE_MYSQL, url, timeout)

	tests := []struct {
		name string
		sql  string
	}{
		{name: "do", sql: "DO SLEEP(10)"},
		{name: "leading comment select", sql: "/* probe */ SELECT SLEEP(10)"},
		{
			name: "cte",
			sql:  "WITH sleeper AS (SELECT SLEEP(10) AS value) SELECT value FROM sleeper",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMySQLClientTimeout(t, drv, tt.sql, timeout)
		})
	}
}
