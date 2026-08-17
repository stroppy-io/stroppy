package driver_test

import (
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// validSource is a one-column indexed source acceptable to ValidateInsert.
func validSource(total int64) *gen.IndexedSource {
	b := gen.NewSchemaBuilder()
	id := b.Int64("id")
	schema := b.Build()

	return gen.NewIndexedSource(schema, gen.Root{}, "test/validate@1", total, 1,
		func(r gen.Row, entity uint64) error {
			r.SetInt64(id, int64(entity))

			return nil
		})
}

func TestValidateInsert(t *testing.T) {
	t.Parallel()

	if err := driver.ValidateInsert(nil); !errors.Is(err, driver.ErrNilInsertRequest) {
		t.Fatalf("nil req: err = %v, want ErrNilInsertRequest", err)
	}

	if err := driver.ValidateInsert(&driver.InsertRequest{
		Table: "t", Method: driver.InsertNative, Workers: 1,
	}); !errors.Is(err, driver.ErrNilInsertSource) {
		t.Fatalf("nil source: err = %v, want ErrNilInsertSource", err)
	}

	empty := gen.NewIndexedSource(
		gen.NewSchemaBuilder().Build(), gen.Root{}, "test/empty@1", 0, 1,
		func(gen.Row, uint64) error { return nil },
	)
	if err := driver.ValidateInsert(&driver.InsertRequest{
		Table: "t", Method: driver.InsertNative, Workers: 1, Source: empty,
	}); !errors.Is(err, gen.ErrEmptySchema) {
		t.Fatalf("empty schema: err = %v, want ErrEmptySchema", err)
	}

	if err := driver.ValidateInsert(&driver.InsertRequest{
		Table: "t", Method: driver.InsertNative, Workers: 1, Source: validSource(10),
	}); err != nil {
		t.Fatalf("valid req: err = %v, want nil", err)
	}
}
