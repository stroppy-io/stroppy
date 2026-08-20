package driver

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/config"
)

// TestInsertMethodsCoverAllDriverTypes guards the matrix against new driver
// types landing in the config enum without a capability row.
func TestInsertMethodsCoverAllDriverTypes(t *testing.T) {
	t.Parallel()

	for _, driverType := range config.DriverTypeValues() {
		methods, ok := insertMethodsByDriver[driverType]
		if !ok {
			t.Errorf("driver type %s has no insert-method capability row", driverType)

			continue
		}

		if len(methods) == 0 {
			t.Errorf("driver type %s declares an empty insert-method list", driverType)
		}
	}
}

func TestInsertMethodsValidAndUnique(t *testing.T) {
	t.Parallel()

	for driverType, methods := range insertMethodsByDriver {
		seen := map[InsertMethod]bool{}

		for _, method := range methods {
			switch method {
			case InsertPlainQuery, InsertPlainBulk, InsertColumnar, InsertNative:
			default:
				t.Errorf("%s: unknown insert method value %d", driverType, method)
			}

			if seen[method] {
				t.Errorf("%s: duplicate insert method %s", driverType, method)
			}

			seen[method] = true
		}
	}
}

func TestInsertCapabilitiesDeterministic(t *testing.T) {
	t.Parallel()

	capabilities := InsertCapabilities()

	if len(capabilities) != len(insertMethodsByDriver) {
		t.Fatalf("expected %d capabilities, got %d",
			len(insertMethodsByDriver), len(capabilities))
	}

	types := make([]config.DriverType, 0, len(capabilities))
	for _, capability := range capabilities {
		types = append(types, capability.Type)
	}

	if !slices.IsSorted(types) {
		t.Errorf("capabilities not ordered by driver enum value: %v", types)
	}
}

func TestInsertMethodsEnumeratesAll(t *testing.T) {
	t.Parallel()

	methods := InsertMethods()
	want := []InsertMethod{InsertPlainQuery, InsertPlainBulk, InsertColumnar, InsertNative}

	if !slices.Equal(methods, want) {
		t.Fatalf("InsertMethods() = %v, want %v", methods, want)
	}
}

func TestResolveInsertMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		driverType config.DriverType
		method     string
		want       InsertMethod
		wantErr    error
		wantErrMsg string
	}{
		{name: "native postgres", driverType: config.DriverTypePostgres, method: "native", want: InsertNative},
		{name: "columnar postgres", driverType: config.DriverTypePostgres, method: "columnar", want: InsertColumnar},
		{name: "empty selects plain_query", driverType: config.DriverTypePostgres, method: "", want: InsertPlainQuery},
		{name: "invalid value", driverType: config.DriverTypePostgres, method: "bogus", wantErr: ErrUnknownInsertMethod},
		{
			name:       "columnar unsupported on mysql",
			driverType: config.DriverTypeMySQL,
			method:     "columnar",
			wantErrMsg: "not supported",
		},
		{
			name:       "native unsupported on csv",
			driverType: config.DriverTypeCSV,
			method:     "plain_bulk",
			wantErrMsg: "not supported",
		},
		{name: "native supported on csv", driverType: config.DriverTypeCSV, method: "native", want: InsertNative},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolveInsertMethod(tc.driverType, tc.method)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ResolveInsertMethod(%q) error = %v, want %v", tc.method, err, tc.wantErr)
				}

				return
			}

			if tc.wantErrMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Fatalf("ResolveInsertMethod(%q) error = %v, want containing %q", tc.method, err, tc.wantErrMsg)
				}

				return
			}

			if err != nil {
				t.Fatalf("ResolveInsertMethod(%q): %v", tc.method, err)
			}

			if got != tc.want {
				t.Fatalf("ResolveInsertMethod(%q) = %v, want %v", tc.method, got, tc.want)
			}
		})
	}
}

func TestSupportsInsertMethod(t *testing.T) {
	t.Parallel()

	if !SupportsInsertMethod(config.DriverTypePostgres, InsertColumnar) {
		t.Fatal("postgres should support columnar")
	}

	if SupportsInsertMethod(config.DriverTypeMySQL, InsertColumnar) {
		t.Fatal("mysql should not support columnar")
	}

	if SupportsInsertMethod(config.DriverTypeCSV, InsertPlainQuery) {
		t.Fatal("csv should only support native")
	}
}
