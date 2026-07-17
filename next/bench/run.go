package bench

// Run is the read-only runtime view of one resolved run. It is what a step's If
// predicate and the query-set resolver read: the resolved seed, the per-slot
// driver kinds, and the env lookup that honors user-provided query-set
// overrides. Define populates slots and baked query-sets; [Main] threads the
// resolved seed in after Define.
type Run struct {
	test  *Test
	seed  uint64
	slots []slotSpec

	getenv func(string) string

	// stdDriverURL/stdDriverKind are the operator-facing standard params for
	// slot 0; [Def.Driver] folds them onto slot 0's declared defaults. They are
	// set by [registerStandardParams] before Define runs.
	stdDriverURL *Param[string]
	stdDriverKind *Param[string]

	// stdInsertMethod is the operator-facing standard param for slot 0's
	// default insert method; [Def.Driver] folds it onto slot 0's declared
	// default so --insert.method reaches a load step that did not pin its own.
	stdInsertMethod *Param[string]

	// retryOpts is the run-level default [RetryOpts], resolved from the
	// --retry.* standard params over the test's [Test.Retry] default.
	// [VU.RetryOpts] hands it to callers that have no per-call override.
	retryOpts RetryOpts

	// bakes holds the baked query-set sources registered by [Def.Queries]; the
	// resolver reads them in declaration order.
	bakes map[string]*BakedQuerySet

	// qset/qOrder memoize each [Def.Queries]/[Run.Queries] resolution so probe
	// can list the exact set an override would target without re-resolving.
	qset   map[string]*resolvedQuerySet
	qOrder []string
}

// Name reports the test name.
func (r *Run) Name() string { return r.test.Name }

// Seed reports the effective run seed (after any -seed override).
func (r *Run) Seed() uint64 { return r.seed }

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
