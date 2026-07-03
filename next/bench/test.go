package bench

import (
	"fmt"
	"hash/fnv"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/driver/noop"
	"github.com/stroppy-io/stroppy/next/driver/pg"
)

// Test is a complete, declarative description of a benchmark: its name, root
// seed, options struct, driver slots and a step-DAG builder. A user test builds
// one and hands it to [Main].
//
// [Main] parses options before it calls Build, so the builder sees fully-parsed
// options and can size executor policies from them (Closed(o.VUs, o.Duration),
// Pool(o.LoadWorkers, ...)) directly — there is no separate options pre-parse.
// Everything else (graph construction, execution) still happens in Main from
// this declarative data, so a Test doubles as the probe/plan description.
type Test struct {
	// Name identifies the test (used in metrics tags and the probe/plan output).
	Name string
	// Seed is the run root seed; the -seed flag overrides it. Together with each
	// step's id it fixes every rng draw (RFC 0001 §5).
	Seed uint64
	// Opts is an optional pointer to a user options struct whose exported fields
	// carry `env:"NAME"` and `default:"..."` tags. Main parses process env into
	// it at startup, before Build, and, if it implements interface{ Validate()
	// error }, calls Validate. Supported field types: string, int, int64,
	// uint64, float64, bool, time.Duration.
	Opts any
	// Drivers declares the database backends by slot. Slot 0 is the default a
	// step's VUs connect to; env overrides apply per slot (see [DriverSlot]).
	Drivers []DriverSlot
	// Build returns the DAG steps in declaration order (used for status
	// reporting and as the deterministic build order; dependencies are declared
	// per step). Main calls it exactly once, after options are parsed and driver
	// slots resolved, passing the [Run] so steps may branch on the resolved
	// options, seed or driver kinds. Steps that need no options can ignore the
	// Run and return a fixed slice.
	Build func(*Run) []*StepDef
}

// DriverSlot declares one database backend. URL and Kind are overridable from
// the environment: slot 0 reads STROPPY_DRIVER_URL / STROPPY_DRIVER_KIND, slot N
// reads STROPPY_DRIVER<N>_URL / STROPPY_DRIVER<N>_KIND. Kind defaults to "pg".
type DriverSlot struct {
	// Name labels the slot so a step's Uses can target it by name.
	Name string
	// Kind selects the driver implementation: "pg" or "noop".
	Kind string
	// URL is the connection string passed to the driver (ignored by noop).
	URL string
}

// slotSpec is a driver slot resolved against the environment: the concrete kind
// and url a run (or a probe) will use.
type slotSpec struct {
	name string
	kind string
	url  string
}

// resolveSlots applies the per-slot env overrides to the declared slots and
// defaults an empty kind to "pg". It does not construct drivers (see
// buildDrivers), so it is safe for -probe/-plan.
func resolveSlots(decls []DriverSlot, getenv func(string) string) []slotSpec {
	out := make([]slotSpec, len(decls))
	for i, d := range decls {
		s := slotSpec{name: d.Name, kind: d.Kind, url: d.URL}
		urlEnv, kindEnv := "STROPPY_DRIVER_URL", "STROPPY_DRIVER_KIND"
		if i > 0 {
			urlEnv = fmt.Sprintf("STROPPY_DRIVER%d_URL", i)
			kindEnv = fmt.Sprintf("STROPPY_DRIVER%d_KIND", i)
		}
		if u := getenv(urlEnv); u != "" {
			s.url = u
		}
		if k := getenv(kindEnv); k != "" {
			s.kind = k
		}
		if s.kind == "" {
			s.kind = "pg"
		}
		out[i] = s
	}
	return out
}

// buildDrivers constructs the concrete driver per slot. It does not connect;
// connections are opened per VU at each step's Init.
func buildDrivers(slots []slotSpec) ([]driver.Driver, error) {
	out := make([]driver.Driver, len(slots))
	for i, s := range slots {
		switch s.kind {
		case "pg":
			out[i] = pg.New(driver.Config{URL: s.url})
		case "noop":
			out[i] = noop.New()
		default:
			return nil, fmt.Errorf("bench: driver slot %d (%q): unknown kind %q", i, s.name, s.kind)
		}
	}
	return out, nil
}

// resolveUses maps a step's Uses slot name to its slot index. An empty name is
// slot 0; an unknown name is an error.
func resolveUses(sd *StepDef, slots []slotSpec) (int, error) {
	if sd.uses == "" {
		return 0, nil
	}
	for i, s := range slots {
		if s.name == sd.uses {
			return i, nil
		}
	}
	return 0, fmt.Errorf("bench: step %q uses unknown driver slot %q", sd.name, sd.uses)
}

// stepID is the rng step id for a step: the 32-bit FNV-1a hash of its name.
//
// Stability contract: a step's id is a pure function of its name — stable across
// runs and independent of the step's position or the presence of other steps.
// Renaming a step changes its id (and thus its rng streams); reordering or
// adding steps does not. Distinct step names (already required by the DAG, which
// rejects duplicate ids) yield distinct step ids barring an astronomically
// unlikely 32-bit hash collision.
func stepID(name string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return h.Sum32()
}
