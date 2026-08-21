package mysql

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
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

func realMySQLConnectionID(t *testing.T, d *Driver) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	value, err := runMySQLFirstValue(ctx, d, "SELECT CAST(CONNECTION_ID() AS CHAR)")
	if err != nil {
		t.Fatalf("read MySQL connection ID: %v", err)
	}

	id, ok := value.(string)
	if !ok {
		t.Fatalf("MySQL connection ID = %#v (%T), want string", value, value)
	}

	return id
}

func assertRealMySQLTimeoutElapsed(t *testing.T, elapsed, timeout time.Duration) {
	t.Helper()

	if minimum := timeout / 2; elapsed < minimum {
		t.Fatalf("query timed out after %s, want at least %s", elapsed, minimum)
	}

	if maximum := timeout + 500*time.Millisecond; elapsed > maximum {
		t.Fatalf("query timed out after %s, want at most %s", elapsed, maximum)
	}
}

func assertRealMySQLServerTimeout(t *testing.T, d *Driver, query string, timeout time.Duration) {
	t.Helper()

	connectionID := realMySQLConnectionID(t, d)
	start := time.Now()
	err := runMySQLExec(context.Background(), d, query)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("query %q returned nil error, want MySQL timeout", query)
	}

	var mysqlErr *gomysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 3024 {
		t.Fatalf("query %q error = %v, want MySQL error 3024", query, err)
	}

	if facts := d.ClassifyError(err); facts.Kind != driver.ErrorKindTimeout {
		t.Fatalf("error kind = %s, want %s", facts.Kind, driver.ErrorKindTimeout)
	}

	if nextID := realMySQLConnectionID(t, d); nextID != connectionID {
		t.Fatalf("connection ID after timeout = %s, want original %s", nextID, connectionID)
	}

	assertRealMySQLTimeoutElapsed(t, elapsed, timeout)
}

func assertRealMySQLClientTimeout(t *testing.T, d *Driver, query string, timeout time.Duration) {
	t.Helper()

	start := time.Now()
	err := runMySQLExec(context.Background(), d, query)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("query %q error = %v, want context.DeadlineExceeded", query, err)
	}

	if facts := d.ClassifyError(err); facts.Kind != driver.ErrorKindTimeout {
		t.Fatalf("error kind = %s, want %s", facts.Kind, driver.ErrorKindTimeout)
	}

	assertRealMySQLTimeoutElapsed(t, elapsed, timeout)
}

func TestStatementTimeoutHintLiveMySQLEligiblePaths(t *testing.T) {
	const timeout = 100 * time.Millisecond

	const expensiveSelect = "COUNT(*) FROM information_schema.columns a " +
		"CROSS JOIN information_schema.columns b CROSS JOIN information_schema.columns c"

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "inserted",
			query: "SELECT " + expensiveSelect,
		},
		{
			name:  "replaced",
			query: "SELECT /*+ MAX_EXECUTION_TIME(5000) */ " + expensiveSelect,
		},
		{
			name:  "canonical unchanged",
			query: "SELECT /*+ MAX_EXECUTION_TIME(100) */ " + expensiveSelect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := realMySQLTimeoutDriver(t, timeout)
			assertRealMySQLServerTimeout(t, d, tt.query, timeout)
		})
	}
}

func TestStatementTimeoutHintLiveMySQLSleepForms(t *testing.T) {
	const timeout = 100 * time.Millisecond

	tests := []struct {
		name  string
		query string
	}{
		{name: "direct", query: "SELECT SLEEP(0.3)"},
		{name: "parenthesized", query: "SELECT (SLEEP(0.3))"},
		{name: "comment prefixed", query: "SELECT /* before */ SLEEP(0.3) /* after */"},
		{
			name:  "comment wrapped parentheses",
			query: "SELECT /* before */ (/* inner */ SLEEP /* call */ (0.3) /* after */)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := realMySQLTimeoutDriver(t, timeout)
			assertRealMySQLClientTimeout(t, d, tt.query, timeout)
		})
	}
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
