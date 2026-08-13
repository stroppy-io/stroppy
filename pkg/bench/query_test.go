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
