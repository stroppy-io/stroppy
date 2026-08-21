//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

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
		Logger:       zap.NewNop(),
		QueryTimeout: timeout,
	})
	if err != nil {
		t.Fatalf("driver.Dispatch(%s): %v", typ, err)
	}

	t.Cleanup(func() { _ = drv.Teardown(context.Background()) })

	return drv
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

// TestQueryTimeoutPostgres blocks pg_sleep past the deadline and verifies the
// statement is cut off as a timeout (pgx CancelRequest → context.DeadlineExceeded)
// rather than a parent cancel, and that the pooled connection is reusable after.
func TestQueryTimeoutPostgres(t *testing.T) {
	skipIfRequested(t)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	drv := dispatchQueryTimeout(t, stroppy.DriverConfig_DRIVER_TYPE_POSTGRES, url, 150*time.Millisecond)

	start := time.Now()
	_, err := drv.RunQuery(context.Background(), "SELECT pg_sleep(10)", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("pg_sleep returned nil error, want timeout")
	}
	if facts := drv.ClassifyError(err); facts.Kind != driver.ErrorKindTimeout {
		t.Fatalf("ClassifyError = %q, want %q (err=%v)", facts.Kind, driver.ErrorKindTimeout, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pg timeout err = %v, want context.DeadlineExceeded", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("pg timeout err = %v was classified as canceled", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("pg_sleep(10) ran %v, deadline did not fire", elapsed)
	}

	assertReusable(t, drv)
}

// TestQueryTimeoutMySQL blocks SELECT SLEEP past the deadline and verifies the
// server-side MAX_EXECUTION_TIME hint (error 3024) fires ahead of the padded
// client deadline, so the connection is not discarded and stays reusable.
func TestQueryTimeoutMySQL(t *testing.T) {
	skipIfRequested(t)

	url := envOr(envMySQLAllURL, defaultMySQLAllURL)
	drv := dispatchQueryTimeout(t, stroppy.DriverConfig_DRIVER_TYPE_MYSQL, url, 150*time.Millisecond)

	start := time.Now()
	_, err := drv.RunQuery(context.Background(), "SELECT SLEEP(10)", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("SELECT SLEEP returned nil error, want timeout")
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
		t.Fatalf("SELECT SLEEP(10) ran %v, deadline did not fire", elapsed)
	}

	assertReusable(t, drv)
}
