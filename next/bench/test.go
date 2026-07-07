package bench

import (
	"fmt"
	"hash/fnv"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/driver/noop"
	"github.com/stroppy-io/stroppy/next/driver/pg"
)

// Test is a complete, declarative description of a benchmark: its name, root
// seed, and one [Def]-based Define callback that registers everything else
// (params, drivers, query-sets, steps, variants, instruments). A user test
// builds one and hands it to [Main].
//
// Define runs against fully-parsed input bags (cli > env > config > default),
// so each declaration resolves immediately: a param's [Param.Value] is the real
// datum, a query-set is parsed, a driver slot knows its kind. Authors derive and
// branch inline. The Test doubles as the probe/plan description — replaying
// Define under the given inputs reproduces the exact plan an operator gets.
type Test struct {
	// Name identifies the test (used in metrics tags and the probe/plan output).
	Name string
	// Seed is the test's spec-representative root seed, as a string. It is the
	// default value of the standard seed param, and the value the seed keywords
	// "fixed"/"canonical" resolve to (F6). The seed param also accepts "auto"/
	// "now" (a fresh random seed per run) or any uint64 literal; --seed overrides
	// all sources. The resolved uint64 root feeds DeriveStream and fixes every
	// rng draw (RFC 0001 §5). An empty Seed defaults to "0" (a valid seed, D11).
	Seed string
	// Define registers the test's params, driver slots, query-sets, steps,
	// variants and instruments against d, using the typed handles D1/D7 provide.
	// It returns a Go native error (no panics, no throw — D10); [Main] surfaces
	// a non-nil return as a configuration failure before any run starts.
	Define func(d *Def) error
}

// slotSpec is a driver slot resolved against the environment: the concrete kind
// and url a run (or a probe) will use.
type slotSpec struct {
	name string
	kind string
	url  string
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
