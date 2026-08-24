package driver

import (
	"slices"
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
