package bench

// Run is the read-only view of a run handed to a step's If predicate. It exposes
// the resolved run parameters — seed, options and per-slot driver kind — so a
// condition can branch on how the run was configured (for example, skipping a
// verification step under the noop driver). It also carries the env lookup so
// [Run.Queries] can honor user-provided query-set overrides, and the memoized
// query-set resolutions so probe can report them.
type Run struct {
	test  *Test
	seed  uint64
	slots []slotSpec

	getenv func(string) string

	qset   map[string]*resolvedQuerySet
	qOrder []string
}

// Name reports the test name.
func (r *Run) Name() string { return r.test.Name }

// Seed reports the effective run seed (after any -seed override).
func (r *Run) Seed() uint64 { return r.seed }

// Opts reports the (parsed) options struct pointer, or nil if the test declares
// none. A predicate type-asserts it back to the test's own options type.
func (r *Run) Opts() any { return r.test.Opts }

// DriverKind reports the resolved kind ("pg"/"noop") of driver slot, or "" if
// the slot index is out of range.
func (r *Run) DriverKind(slot int) string {
	if slot < 0 || slot >= len(r.slots) {
		return ""
	}
	return r.slots[slot].kind
}

// DriverKindByName reports the resolved kind of the named slot, or "" if no slot
// has that name.
func (r *Run) DriverKindByName(name string) string {
	for _, s := range r.slots {
		if s.name == name {
			return s.kind
		}
	}
	return ""
}
