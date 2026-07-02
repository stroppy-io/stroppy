package driver

import (
	"slices"
	"testing"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// TestInsertMethodsCoverAllDriverTypes guards the matrix against new driver
// types landing in the proto enum without a capability row.
func TestInsertMethodsCoverAllDriverTypes(t *testing.T) {
	t.Parallel()

	for value, name := range stroppy.DriverConfig_DriverType_name {
		driverType := stroppy.DriverConfig_DriverType(value)
		if driverType == stroppy.DriverConfig_DRIVER_TYPE_UNSPECIFIED {
			continue
		}

		methods, ok := insertMethodsByDriver[driverType]
		if !ok {
			t.Errorf("driver type %s has no insert-method capability row", name)

			continue
		}

		if len(methods) == 0 {
			t.Errorf("driver type %s declares an empty insert-method list", name)
		}
	}
}

func TestInsertMethodsValidAndUnique(t *testing.T) {
	t.Parallel()

	for driverType, methods := range insertMethodsByDriver {
		seen := map[dgproto.InsertMethod]bool{}

		for _, method := range methods {
			if _, ok := dgproto.InsertMethod_name[int32(method)]; !ok {
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

	types := make([]stroppy.DriverConfig_DriverType, 0, len(capabilities))
	for _, capability := range capabilities {
		types = append(types, capability.Type)
	}

	if !slices.IsSorted(types) {
		t.Errorf("capabilities not ordered by driver enum value: %v", types)
	}
}
