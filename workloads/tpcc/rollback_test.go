package tpcc

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestProtocolRollbackSentinel(t *testing.T) {
	pg := &pgconn.PgError{Code: "P0001", Message: errRollbackSentinel.Error()}

	my := &mysql.MySQLError{Number: 1644, SQLState: [5]byte{'4', '5', '0', '0', '0'}, Message: errRollbackSentinel.Error()}
	for name, err := range map[string]error{"native": errRollbackSentinel, "postgres": pg, "mysql": my} {
		t.Run(name, func(t *testing.T) {
			wrapped := fmt.Errorf("new_order: %w", err)
			if !isRollbackSentinel(wrapped) || finishNewOrder(wrapped, nil) != nil {
				t.Fatal("expected rollback was reported as a transaction failure")
			}

			if !isRollbackSentinel(fmt.Errorf("closed rows: %w", errors.Join(wrapped))) {
				t.Fatal("a single deduplicated row error must retain rollback identity")
			}

			failed := errors.New("rollback connection lost")
			if got := finishNewOrder(wrapped, failed); !errors.Is(got, failed) || isRollbackSentinel(got) {
				t.Fatalf("rollback failure must remain an error: %v", got)
			}

			if isRollbackSentinel(errors.Join(wrapped, failed)) {
				t.Fatal("joined failure was mistaken for a successful rollback")
			}

			if isRollbackSentinel(fmt.Errorf("wrapped join: %w", errors.Join(wrapped, failed))) {
				t.Fatal("wrapped joined failure was mistaken for a successful rollback")
			}
		})
	}

	for _, err := range []error{
		nil,
		errors.New("tpcc_rollback:unexpected_failure"),
		&pgconn.PgError{Code: "40001", Message: errRollbackSentinel.Error()},
		&pgconn.PgError{Code: "P0001", Message: "unexpected failure"},
		&mysql.MySQLError{Number: 1213, Message: errRollbackSentinel.Error()},
	} {
		if isRollbackSentinel(err) {
			t.Fatalf("unrelated error accepted as a rollback: %v", err)
		}
	}
}
