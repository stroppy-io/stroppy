package csv

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
		{name: "query unsupported", err: ErrCsvDriverNoQuery, want: driver.ErrorKindUnsupported},
		{name: "wrapped insert unsupported", err: fmt.Errorf("insert: %w", ErrUnsupportedInsertMethod), want: driver.ErrorKindUnsupported},
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
