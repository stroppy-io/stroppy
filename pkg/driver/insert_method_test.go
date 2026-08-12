package driver_test

import (
	"testing"

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

func TestInsertMethodString(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		m    driver.InsertMethod
		want string
	}{
		{driver.InsertPlainQuery, "plain_query"},
		{driver.InsertPlainBulk, "plain_bulk"},
		{driver.InsertColumnar, "columnar"},
		{driver.InsertNative, "native"},
	} {
		if got := c.m.String(); got != c.want {
			t.Fatalf("%v.String() = %q, want %q", c.m, got, c.want)
		}
	}
}
