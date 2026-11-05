package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want driver.ErrorKind
	}{
		{name: "serialization", err: &pgconn.PgError{Code: "40001"}, want: driver.ErrorKindSerialization},
		{
			name: "wrapped deadlock",
			err:  fmt.Errorf("query: %w", &pgconn.PgError{Code: "40P01"}),
			want: driver.ErrorKindDeadlock,
		},
		{name: "lock timeout", err: &pgconn.PgError{Code: "55P03"}, want: driver.ErrorKindLockTimeout},
		{name: "query canceled", err: &pgconn.PgError{Code: "57014"}, want: driver.ErrorKindTimeout},
		{
			name: "parent cancellation wins over server cancellation",
			err:  errors.Join(context.Canceled, &pgconn.PgError{Code: "57014"}),
			want: driver.ErrorKindCanceled,
		},
		{
			name: "deadline remains timeout",
			err:  errors.Join(context.DeadlineExceeded, &pgconn.PgError{Code: "57014"}),
			want: driver.ErrorKindTimeout,
		},
		{name: "unique", err: &pgconn.PgError{Code: "23505"}, want: driver.ErrorKindUniqueViolation},
		{name: "foreign key", err: &pgconn.PgError{Code: "23503"}, want: driver.ErrorKindConstraintViolation},
		{name: "not null", err: &pgconn.PgError{Code: "23502"}, want: driver.ErrorKindConstraintViolation},
		{name: "check", err: &pgconn.PgError{Code: "23514"}, want: driver.ErrorKindConstraintViolation},
		{name: "other postgres", err: &pgconn.PgError{Code: "XX000"}, want: driver.ErrorKindUnknown},
		{name: "other", err: errors.New("boom"), want: driver.ErrorKindUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := (*Driver)(nil).ClassifyError(tt.err).Kind; got != tt.want {
				t.Fatalf("ClassifyError().Kind = %q, want %q", got, tt.want)
			}
		})
	}
}
