package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestInsertRejectsNilRequest(t *testing.T) {
	t.Parallel()

	_, err := (&Bench{}).Insert(context.Background(), nil)
	if !errors.Is(err, driver.ErrNilInsertRequest) {
		t.Fatalf("Insert error = %v, want ErrNilInsertRequest", err)
	}
}

type errorRows struct {
	err error
}

func (*errorRows) Columns() []string   { return nil }
func (*errorRows) Next() bool          { return false }
func (*errorRows) Values() []any       { return nil }
func (*errorRows) ReadAll(int) [][]any { return nil }
func (r *errorRows) Err() error        { return r.err }
func (*errorRows) Close() error        { return nil }

func TestFirstQueryValueReturnsRowError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("rows")

	value, err := firstQueryValue(&errorRows{err: sentinel})
	if value != nil {
		t.Fatalf("firstQueryValue() value = %v, want nil", value)
	}

	if !errors.Is(err, sentinel) {
		t.Fatalf("firstQueryValue() error = %v, want sentinel", err)
	}
}
