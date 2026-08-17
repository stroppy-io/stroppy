package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want driver.ErrorKind
	}{
		{name: "serialization", err: &pgconn.PgError{Code: "40001"}, want: driver.ErrorKindSerialization},
		{name: "wrapped deadlock", err: fmt.Errorf("query: %w", &pgconn.PgError{Code: "40P01"}), want: driver.ErrorKindDeadlock},
		{name: "other postgres", err: &pgconn.PgError{Code: "23505"}, want: driver.ErrorKindUnknown},
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
