package picodata

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want driver.ErrorKind
	}{
		{name: "transactions unsupported", err: ErrTransactionsUnsupported, want: driver.ErrorKindUnsupported},
		{name: "wrapped native unsupported", err: fmt.Errorf("insert: %w", ErrNativeUnsupported), want: driver.ErrorKindUnsupported},
		{name: "unsupported type", err: ErrUnsupportedType, want: driver.ErrorKindUnsupported},
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
