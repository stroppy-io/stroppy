package driver

import (
	"slices"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// InsertCapability pairs a driver type with the insert methods its
// implementation serves.
type InsertCapability struct {
	Type          stroppy.DriverConfig_DriverType
	InsertMethods []InsertMethod
}

// insertMethodsByDriver is the static driver→insert-method matrix. It is
// declared here rather than registered from the driver packages because the
// stroppy CLI links only this package — probe must answer offline without
// pulling driver implementations (and their database clients) into the
// binary. Keep each row in sync with the method switch in
// pkg/driver/<type>.
//
//nolint:exhaustive // DRIVER_TYPE_UNSPECIFIED deliberately has no capability row.
var insertMethodsByDriver = map[stroppy.DriverConfig_DriverType][]InsertMethod{
	stroppy.DriverConfig_DRIVER_TYPE_POSTGRES: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertColumnar,
		InsertNative,
	},
	stroppy.DriverConfig_DRIVER_TYPE_MYSQL: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertNative,
	},
	stroppy.DriverConfig_DRIVER_TYPE_PICODATA: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertNative,
	},
	stroppy.DriverConfig_DRIVER_TYPE_YDB: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertColumnar,
		InsertNative,
	},
	stroppy.DriverConfig_DRIVER_TYPE_NOOP: {
		InsertPlainQuery,
		InsertPlainBulk,
		InsertColumnar,
		InsertNative,
	},
	stroppy.DriverConfig_DRIVER_TYPE_CSV: {
		InsertNative,
	},
}

// InsertCapabilities returns the driver→insert-method matrix ordered by
// driver enum value. Method lists follow enum value order too, so output
// is deterministic for machine consumers.
func InsertCapabilities() []InsertCapability {
	types := make([]stroppy.DriverConfig_DriverType, 0, len(insertMethodsByDriver))
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
