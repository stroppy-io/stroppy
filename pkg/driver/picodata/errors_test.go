package picodata

import (
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
		{name: "transactions unsupported", err: ErrTransactionsUnsupported, want: driver.ErrorKindUnsupported},
		{
			name: "wrapped native unsupported",
			err:  fmt.Errorf("insert: %w", ErrNativeUnsupported),
			want: driver.ErrorKindUnsupported,
		},
		{name: "unsupported type", err: ErrUnsupportedType, want: driver.ErrorKindUnsupported},
		{
			name: "wrapped duplicate",
			err: fmt.Errorf("query: %w", &pgconn.PgError{
				Code: "XX000", Message: `sbroad: transaction rolled-back: TupleFound: Duplicate key exists in unique index "pkey"`,
			}),
			want: driver.ErrorKindUniqueViolation,
		},
		{name: "unique code", err: &pgconn.PgError{Code: "23505"}, want: driver.ErrorKindUniqueViolation},
		{name: "foreign key", err: &pgconn.PgError{Code: "23503"}, want: driver.ErrorKindConstraintViolation},
		{
			name: "untyped duplicate text", err: errors.New("TupleFound: Duplicate key exists in unique index"),
			want: driver.ErrorKindUnknown,
		},
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
