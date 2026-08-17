package mysql

import (
	"errors"
	"fmt"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want driver.ErrorKind
	}{
		{name: "deadlock", err: &gomysql.MySQLError{Number: errLockDeadlock}, want: driver.ErrorKindDeadlock},
		{name: "wrapped lock timeout", err: fmt.Errorf("query: %w", &gomysql.MySQLError{Number: errLockWaitTimeout}), want: driver.ErrorKindLockTimeout},
		{name: "other mysql", err: &gomysql.MySQLError{Number: 1062}, want: driver.ErrorKindUnknown},
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
