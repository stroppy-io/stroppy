package mysql

import (
	"errors"
	"fmt"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want driver.ErrorKind
	}{
		{name: "deadlock", err: &gomysql.MySQLError{Number: errLockDeadlock}, want: driver.ErrorKindDeadlock},
		{
			name: "wrapped snapshot conflict",
			err: fmt.Errorf("transaction: %w", &gomysql.MySQLError{
				Number:   1020,
				SQLState: [5]byte{'H', 'Y', '0', '0', '0'}, Message: "Record has changed since last read",
			}),
			want: driver.ErrorKindSerialization,
		},
		{
			name: "wrapped lock timeout",
			err:  fmt.Errorf("query: %w", &gomysql.MySQLError{Number: errLockWaitTimeout}),
			want: driver.ErrorKindLockTimeout,
		},
		{name: "query timeout", err: &gomysql.MySQLError{Number: errQueryTimeout}, want: driver.ErrorKindTimeout},
		{name: "unique", err: &gomysql.MySQLError{Number: 1062}, want: driver.ErrorKindUniqueViolation},
		{name: "foreign key", err: &gomysql.MySQLError{Number: 1452}, want: driver.ErrorKindConstraintViolation},
		{name: "not null", err: &gomysql.MySQLError{Number: 1048}, want: driver.ErrorKindConstraintViolation},
		{name: "check", err: &gomysql.MySQLError{Number: 3819}, want: driver.ErrorKindConstraintViolation},
		{
			name: "MariaDB check", err: &gomysql.MySQLError{Number: errMariaDBCheckConstraint},
			want: driver.ErrorKindConstraintViolation,
		},
		{name: "other mysql", err: &gomysql.MySQLError{Number: 1105}, want: driver.ErrorKindUnknown},
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
