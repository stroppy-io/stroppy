package driver_test

import (
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestParseInsertMethod(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want driver.InsertMethod
	}{
		{"", driver.InsertPlainQuery},
		{"plain_query", driver.InsertPlainQuery},
		{"plain_bulk", driver.InsertPlainBulk},
		{"columnar", driver.InsertColumnar},
		{"native", driver.InsertNative},
	}
	for _, c := range cases {
		got, err := driver.ParseInsertMethod(c.in)
		if err != nil {
			t.Fatalf("ParseInsertMethod(%q): %v", c.in, err)
		}

		if got != c.want {
			t.Fatalf("ParseInsertMethod(%q) = %v, want %v", c.in, got, c.want)
		}

		if got.String() == "" {
			t.Fatalf("String() empty for %q", c.in)
		}
	}

	if _, err := driver.ParseInsertMethod("bogus"); err == nil {
		t.Fatalf("unknown method did not error")
	}
}

func TestMethodProtoRoundTrip(t *testing.T) {
	t.Parallel()

	for _, m := range []driver.InsertMethod{
		driver.InsertPlainQuery,
		driver.InsertPlainBulk,
		driver.InsertColumnar,
		driver.InsertNative,
	} {
		if driver.MethodFromProto(driver.MethodToProto(m)) != m {
			t.Fatalf("round-trip lost for %v", m)
		}
	}
	// The zero/unknown proto value maps to the zero InsertMethod.
	var zero driver.InsertMethod
	if driver.MethodFromProto(dgproto.InsertMethod(999)) != zero {
		t.Fatalf("unknown proto did not map to zero")
	}
}
