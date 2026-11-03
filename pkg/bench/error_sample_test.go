package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestRepresentativeError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		code string
		want string
	}{
		{"postgres", fmt.Errorf("SELECT secret args [secret]: %w", &pgconn.PgError{
			Code: "23505", Message: `duplicate key value violates unique constraint "secret_index"`,
			Detail: "Key (password)=(secret) already exists", Where: "SELECT secret",
		}), "23505", "duplicate key value violates unique constraint <redacted>"},
		{
			"mysql", &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'secret' for key 'PRIMARY'"},
			"1062", "Duplicate entry <redacted> for key <redacted>",
		},
		{
			"wrapped unknown", fmt.Errorf("query SELECT secret: %w", errors.New("backend unavailable")),
			"", "backend unavailable",
		},
		{"arguments", errors.New("backend failure; args [secret]"), "", "backend failure; <arguments omitted>"},
		{"sql", errors.New("backend failure: SELECT secret FROM table"), "", "<SQL omitted>"},
		{
			"replace", errors.New("backend failure: REPLACE INTO users VALUES (12345)"),
			"", "backend failure: <SQL omitted>",
		},
		{"configured secret", errors.New("authentication failed secret"), "", "authentication failed <redacted>"},
		{
			"serialization", &pgconn.PgError{Code: "40001", Message: "could not serialize access due to concurrent update"},
			"40001", "could not serialize access due to concurrent update",
		},
		{
			"foreign key", &mysql.MySQLError{
				Number:  1452,
				Message: "Cannot add or update a child row: a foreign key constraint fails ('secret')",
			},
			"1452", "Cannot add or update a child row: a foreign key constraint fails (<redacted>)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, sample := representativeError(tc.err, strings.NewReplacer("secret", "<redacted>"))
			require.Equal(t, tc.code, code)
			require.Equal(t, tc.want, sample)
			require.NotContains(t, sample, "secret")
		})
	}
}

func TestErrorSampleRedactionCannotDisableSQLOmission(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		secret  string
		message string
	}{
		{"SET", "execution failed: UPDATE users SET password=12345"},
		{"UPDATE", "execution failed: UPDATE users SET password=12345"},
		{"'prefix' SELECT", "failure 'prefix' SELECT private_suffix FROM users"},
		{"prefix'quote", "failure prefix'quote private_suffix: REPLACE INTO users VALUES (12345)"},
	} {
		t.Run(tc.secret, func(t *testing.T) {
			t.Parallel()

			_, sample := representativeError(errors.New(tc.message), strings.NewReplacer(tc.secret, "<redacted>"))
			require.Equal(t, "<SQL omitted>", sample)
		})
	}
}

func TestErrorReporterRetainsBoundedFirstSample(t *testing.T) {
	t.Parallel()

	reporter := newErrorReporter(zap.NewNop(), time.Hour)
	t.Cleanup(reporter.stopAndWait)

	first := "first " + strings.Repeat(string(rune(0x754c)), maxErrorSampleBytes)
	reporter.record(nil, terminalErrorIteration, "workload", &pgconn.PgError{Code: "XX000", Message: first}, nil)

	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			reporter.record(nil, terminalErrorIteration, "workload", errors.New("later error"), nil)
		})
	}

	workers.Wait()

	summary := reporter.snapshot()
	require.Len(t, summary.groups, 1)
	require.Equal(t, uint64(17), summary.groups[0].count)
	require.True(t, strings.HasPrefix(summary.groups[0].sample, "first "))
	require.LessOrEqual(t, len(summary.groups[0].sample), maxErrorSampleBytes)
	require.True(t, utf8.ValidString(summary.groups[0].sample))
	converted := reportErrors(summary)
	require.Equal(t, summary.groups[0].sample, converted.Groups[0].Sample)
	require.Equal(t, "XX000", converted.Groups[0].Code)
	data, err := json.Marshal(converted)
	require.NoError(t, err)
	require.Contains(t, string(data), `"code":"XX000"`)

	var output bytes.Buffer
	reporter.writeSummary(&output)
	require.Contains(t, output.String(), "sample=\"first ")
}

func TestConstraintErrorsRemainTerminalAndContinue(t *testing.T) {
	t.Parallel()

	reporter := newErrorReporter(zap.NewNop(), time.Hour)
	t.Cleanup(reporter.stopAndWait)

	for _, kind := range []driver.ErrorKind{driver.ErrorKindUniqueViolation, driver.ErrorKindConstraintViolation} {
		policy := newTxRetryPolicy(func(error) driver.ErrorFacts { return driver.ErrorFacts{Kind: kind} },
			TxRetryPolicyOptions{MaxAttempts: 3})
		attempts := 0
		err := Retry0(context.Background(), policy, func() error {
			attempts++

			return errors.New("constraint rejected")
		})
		require.Error(t, err)
		require.False(t, IsFatalError(err))
		require.Equal(t, 1, attempts)
		reporter.record(nil, terminalErrorIteration, "workload", err, policy.Classify)
	}

	require.Equal(t, uint64(2), reporter.snapshot().failedIterations)
	require.Len(t, reporter.snapshot().groups, 2)
}
