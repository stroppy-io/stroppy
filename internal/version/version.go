package version

import "runtime/debug"

const modulePath = "github.com/stroppy-io/stroppy/v6"

var Version = "unknown"

// Resolve returns the injected Stroppy version, or the linked module version
// when Stroppy runs as a dependency of a standalone workload.
func Resolve() string {
	if Version != "" && Version != "unknown" {
		return Version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}

	if info.Main.Path == modulePath && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	for _, dependency := range info.Deps {
		if dependency.Path == modulePath {
			return dependency.Version
		}
	}

	return Version
}
