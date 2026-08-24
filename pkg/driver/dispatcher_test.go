package driver

import (
	"context"
	"errors"
	"testing"
)

func TestDispatchNilConfig(t *testing.T) {
	t.Parallel()

	driver, err := Dispatch(context.Background(), Options{})
	if driver != nil {
		t.Fatalf("Dispatch() driver = %v, want nil", driver)
	}

	if err == nil || !errors.Is(err, ErrNoRegisteredDriver) {
		t.Fatalf("Dispatch() error = %v, want ErrNoRegisteredDriver", err)
	}
}
