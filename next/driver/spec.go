package driver

import "time"

// Acquisition selects how a slot's connections reach its driver (D2/F2).
type Acquisition uint8

const (
	// PerVU is the default acquisition: each VU opens one dedicated connection
	// pinned for its whole lifetime, so the measured path has no pool
	// contention (RFC 0001 §10). Pool bounds in [Spec] are unused.
	PerVU Acquisition = iota
	// Shared lends borrowed connections from one connection pool shared across
	// every VU of the slot's step. A VU acquires a connection per use and
	// returns it on Close. Pool bounds ([Spec.MinConns]/[Spec.MaxConns]) bound
	// the pool. Use Shared for non-measured slots where contention is
	// acceptable — multi-driver setups, background work — never for the
	// measured path a closed-loop workload drives.
	Shared
)

// String renders the acquisition mode for probe/diagnostics.
func (a Acquisition) String() string {
	switch a {
	case Shared:
		return "shared"
	default:
		return "per-vu"
	}
}

// FieldSource names the provenance of one resolved [Spec] field, so the probe
// can show which layer set the effective value (D2): pinned by the test,
// derived by the test from resolved params, or the SDK/driver default.
type FieldSource uint8

const (
	// FieldDefault is the SDK/driver default value.
	FieldDefault FieldSource = iota
	// FieldDerived marks a value the test computed from other resolved params
	// (a soft suggestion; an operator override would still win where supported).
	FieldDerived
	// FieldPinned marks a hard requirement the test enforces (wins over an
	// operator override), e.g. a single-connection test pinning pool=1.
	FieldPinned
)

// String renders the field source for probe/diagnostics.
func (s FieldSource) String() string {
	switch s {
	case FieldDerived:
		return "derived"
	case FieldPinned:
		return "pinned"
	default:
		return "default"
	}
}

// Spec is the resolved configuration a concrete driver is constructed from. It
// carries the connection URL, the acquisition mode, pool bounds (meaningful
// only for [Shared] acquisition), a connect timeout, and Native — an opaque map
// of driver-specific advanced knobs (D2 class B) a per-dbdrv typed accessor
// reads (see e.g. [github.com/stroppy-io/stroppy/next/driver/pg.Native]).
//
// Field provenance is recorded in Sources so the probe can show, per pool bound
// or native knob, whether it was the default, derived from params, or pinned by
// the test (D2 introspection: "pool=1 — pinned by test").
type Spec struct {
	// URL is the driver-specific connection string (e.g. a libpq/pgx DSN). Auth
	// and TLS fold into it: pgx parses both from the URL, so the Spec carries
	// no separate auth/TLS fields (D2: keep the boundary clean).
	URL string
	// MinConns and MaxConns bound a connection pool when Mode == Shared; a
	// PerVU driver ignores them. MaxConns <= 0 lets the driver pick its
	// default.
	MinConns int32
	MaxConns int32
	// ConnectTimeout caps a single Connect/Acquire. Zero means the driver
	// default.
	ConnectTimeout time.Duration
	// Mode selects PerVU (pinned, default) or Shared (pooled) acquisition.
	Mode Acquisition
	// InsertMethod is the slot's resolved default insert method, inherited by a
	// load step that does not pin its own. The zero value ([InsertNative]) lets
	// each driver pick its fastest path. An operator's --insert.method override
	// folds into slot 0 here at resolution time.
	InsertMethod InsertMethod
	// Native holds driver-specific advanced knobs (D2 class B). Opaque to the
	// SDK; a per-dbdrv typed accessor (pg.Native) reads it.
	Native map[string]any
	// Sources records the provenance of selected fields ("min", "max", "mode")
	// for probe/diagnostics. Nil entries mean FieldDefault.
	Sources map[string]FieldSource
}

// source returns the recorded provenance of field, or FieldDefault when unset.
func (s Spec) source(field string) FieldSource {
	if s.Sources == nil {
		return FieldDefault
	}
	return s.Sources[field]
}

// SetSource records the provenance of field. Spec is a value type, so callers
// mutate through a pointer (the bench layer holds *DriverSpec and calls this
// through the addressable spec field).
func (s *Spec) SetSource(field string, src FieldSource) {
	if s.Sources == nil {
		s.Sources = make(map[string]FieldSource)
	}
	s.Sources[field] = src
}

// SetNative stores val under key in the Native map, allocating it on first use.
func (s *Spec) SetNative(key string, val any) {
	if s.Native == nil {
		s.Native = make(map[string]any)
	}
	s.Native[key] = val
}
