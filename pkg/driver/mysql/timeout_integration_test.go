package mysql

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

const realMySQLDSNEnv = "STROPPY_MYSQL_DSN"

func realMySQLTimeoutDriver(t *testing.T, timeout time.Duration) *Driver {
	t.Helper()

	dsn := os.Getenv(realMySQLDSNEnv)
	if dsn == "" {
		t.Skip(realMySQLDSNEnv + " not set; skipping real-MySQL timeout test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d, err := NewDriver(ctx, driver.Options{
		Logger: zap.NewNop(),
		Config: &stroppy.DriverConfig{
			Url:        dsn,
			DriverType: stroppy.DriverConfig_DRIVER_TYPE_MYSQL,
		},
		QueryTimeout: timeout,
	})
	if err != nil {
		t.Fatalf("create MySQL driver: %v", err)
	}

	d.db.SetMaxOpenConns(1)
	d.db.SetMaxIdleConns(1)

	t.Cleanup(func() {
		teardownCtx, teardownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer teardownCancel()

		if err := d.Teardown(teardownCtx); err != nil {
			t.Errorf("tear down MySQL driver: %v", err)
		}
	})

	return d
}

func execRealMySQL(t *testing.T, d *Driver, query string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := d.db.ExecContext(ctx, query); err != nil {
		t.Fatalf("execute MySQL fixture query: %v", err)
	}
}

func finishMySQLQuery(res *driver.QueryResult, queryErr error) error {
	if res == nil || res.Rows == nil {
		return queryErr
	}

	closeErr := res.Rows.Close()

	return errors.Join(queryErr, res.Rows.Err(), closeErr)
}

func runMySQLExec(ctx context.Context, d *Driver, query string) (err error) {
	res, err := d.RunQuery(ctx, query, nil)
	defer func() { err = finishMySQLQuery(res, err) }()

	return err
}

func runMySQLFirstValue(ctx context.Context, d *Driver, query string) (_ any, err error) {
	res, err := d.RunQuery(ctx, query, nil)
	defer func() { err = finishMySQLQuery(res, err) }()

	if err != nil {
		return nil, err
	}

	if !res.Rows.Next() {
		return nil, res.Rows.Err()
	}

	values := res.Rows.Values()
	if len(values) == 0 {
		return nil, res.Rows.Err()
	}

	return values[0], res.Rows.Err()
}

func TestProcedureResultCleanupHonorsQueryTimeout(t *testing.T) {
	const (
		procedure = "stroppy_result_cleanup_timeout"
		timeout   = 100 * time.Millisecond
	)

	d := realMySQLTimeoutDriver(t, timeout)
	execRealMySQL(t, d, "DROP PROCEDURE IF EXISTS "+procedure)
	execRealMySQL(t, d, "CREATE PROCEDURE "+procedure+"() BEGIN SELECT 1; DO SLEEP(2); END")
	t.Cleanup(func() { execRealMySQL(t, d, "DROP PROCEDURE IF EXISTS "+procedure) })

	tests := []struct {
		name     string
		firstRow bool
		run      func(context.Context) (any, error)
	}{
		{
			name: "exec",
			run: func(ctx context.Context) (any, error) {
				return nil, runMySQLExec(ctx, d, "CALL "+procedure+"()")
			},
		},
		{
			name:     "first_row",
			firstRow: true,
			run: func(ctx context.Context) (any, error) {
				return runMySQLFirstValue(ctx, d, "CALL "+procedure+"()")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			value, err := tt.run(context.Background())
			elapsed := time.Since(start)

			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("procedure error = %v, want context.DeadlineExceeded", err)
			}

			if facts := d.ClassifyError(err); facts.Kind != driver.ErrorKindTimeout {
				t.Fatalf("error kind = %s, want %s", facts.Kind, driver.ErrorKindTimeout)
			}

			if elapsed >= time.Second {
				t.Fatalf("procedure cleanup took %s, want less than 1s", elapsed)
			}

			if tt.firstRow && value != "1" && value != int64(1) {
				t.Fatalf("first value = %#v, want 1", value)
			}

			followupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			value, err = runMySQLFirstValue(followupCtx, d, "SELECT 1")
			if err != nil {
				t.Fatalf("follow-up query error = %v", err)
			}

			if value != "1" && value != int64(1) {
				t.Fatalf("follow-up value = %#v, want 1", value)
			}
		})
	}
}
