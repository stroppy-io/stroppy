package driver

import (
	"errors"
	"fmt"
	"slices"

	"github.com/stroppy-io/stroppy/pkg/config"
)

// InsertCapability pairs a driver type with the insert methods its
// implementation serves.
type InsertCapability struct {
	Type          config.DriverType
	InsertMethods []InsertMethod
}

// insertMethodsByDriver is the static driver→insert-method matrix. It is
// declared here rather than registered from the driver packages because the
// stroppy CLI links only this package — probe must answer offline without
// pulling driver implementations (and their database clients) into the
// binary. Keep each row in sync with the method switch in
// pkg/driver/<type>.
//
//nolint:exhaustive // DriverTypeUnspecified deliberately has no capability row.
var insertMethodsByDriver = map[config.DriverType][]InsertMethod{
	config.DriverTypePostgres: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertColumnar,
		InsertNative,
	},
	config.DriverTypeMySQL: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertNative,
	},
	config.DriverTypePicodata: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertNative,
	},
	config.DriverTypeYDB: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertColumnar,
		InsertNative,
	},
	config.DriverTypeNoop: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertColumnar,
		InsertNative,
	},
	config.DriverTypeCSV: {
		InsertNative,
	},
}

// InsertCapabilities returns the driver→insert-method matrix ordered by
// driver enum value. Method lists follow enum value order too, so output
// is deterministic for machine consumers.
func InsertCapabilities() []InsertCapability {
	types := make([]config.DriverType, 0, len(insertMethodsByDriver))
	for driverType := range insertMethodsByDriver {
		types = append(types, driverType)
	}

	slices.Sort(types)

	capabilities := make([]InsertCapability, 0, len(types))
	for _, driverType := range types {
		capabilities = append(capabilities, InsertCapability{
			Type:          driverType,
			InsertMethods: slices.Clone(insertMethodsByDriver[driverType]),
		})
	}

	return capabilities
}

// InsertMethods returns every supported insert method in enum order. It is the
// authoritative value set probe and workload help enumerate.
func InsertMethods() []InsertMethod {
	return []InsertMethod{InsertPlainQuery, InsertPlainBulk, InsertColumnar, InsertNative}
}

// ErrInsertMethodUnsupported is returned when a resolved insert method is not
// served by the selected driver.
var ErrInsertMethodUnsupported = errors.New("insert method not supported by driver")

// SupportsInsertMethod reports whether driverType's implementation serves method.
func SupportsInsertMethod(driverType config.DriverType, method InsertMethod) bool {
	return slices.Contains(insertMethodsByDriver[driverType], method)
}

// ResolveInsertMethod parses an authoring string and rejects methods the
// selected driver does not serve, so an invalid or unsupported value fails
// before a run starts loading.
func ResolveInsertMethod(driverType config.DriverType, s string) (InsertMethod, error) {
	method, err := ParseInsertMethod(s)
	if err != nil {
		return 0, err
	}

	if !SupportsInsertMethod(driverType, method) {
		return 0, fmt.Errorf("%w %q (%s driver)", ErrInsertMethodUnsupported, s, driverType)
	}

	return method, nil
}
