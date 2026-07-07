package bench

import (
	"fmt"

	"github.com/stroppy-io/stroppy/next/metrics"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// Def is the immediate-mode declaration context handed to a test's Define
// callback. Each method both registers a declaration (for introspection) and
// resolves it on the spot (typed param values, eager query-set resolution,
// driver slot specs), so an author writes straight-line Go: derive, branch,
// size executor policies from already-resolved params — no separate pre-parse,
// no deferred wiring. The SDK's phase model (parse bags -> Define -> freeze +
// build + run) keeps this immediate resolution safe: Define is pure declaration
// + derivation against resolved inputs; it does not connect, freeze, or run.
//
// The one discipline: declare param existence unconditionally at the top, so
// discovery (probe/plan/help) never lies — a param declared inside an untaken
// branch is invisible to introspection. Derivation and branching belong on
// steps, variants, and magnitudes, never on whether a param exists.
type Def struct {
	// Param is the typed param registry: declare a param and receive its handle
	// with the value already resolved (cli > env > config > default). Authors
	// read .Value() to derive and branch inline.
	Param *ParamSet

	test   *Test
	run    *Run // runtime view being populated (slots, bakes, query resolutions)
	set    *ParamSet
	getenv func(string) string

	drivers []*DriverSpec
	drvName map[string]*DriverSpec

	steps      []*StepDef
	stepByName map[string]*StepDef

	variants []variantDef
	varByName map[string]*variantDef

	instruments []*instrumentDecl
}

// variantDef is one declared variant: a name plus the step set it admits. A nil
// step set means "all declared steps" (D3b: empty = all).
type variantDef struct {
	name  string
	steps map[string]bool // nil/empty = all
}

// instrumentDecl is one author-declared metric, recorded at Define and
// registered with the metrics.Registry at phase 3 (D6 populates the
// registration; until then the handle's [Histogram.Handle]/[Counter.Handle]
// panics, since the underlying MetricHandle is not yet assigned).
type instrumentDecl struct {
	kind  instrumentKind
	name  string
	tags  []instrumentTag
	handle *metricsHandle // the forward-ref handle assigned at phase 3
}

type instrumentKind uint8

const (
	kindHistogram instrumentKind = iota
	kindCounter
)

// instrumentTag is one tag column of an author instrument (D6 tag columns).
// Recorded now so phase-3 registration has the full fan-out; the value space is
// resolved by D6.
type instrumentTag struct {
	name string
}

// metricsHandle is the shared forward-ref state of a Histogram/Counter: the
// underlying metrics handle is nil until phase-3 registration assigns it (D6).
type metricsHandle struct {
	assigned bool
	mh       metrics.MetricHandle // valid only when assigned
	ch       metrics.CounterHandle
	isHist   bool
}

// Seed reports the resolved run root seed (the uint64 fed to DeriveStream).
// It is available inside Define so an author can size eagerly-built run-global
// state (e.g. a generation world) from it; the value is fixed before Define
// runs. For an invalid seed string the value is 0 and the error is surfaced
// separately by [Main] after Define.
func (d *Def) Seed() uint64 { return d.run.seed }

// Driver declares one database backend slot and returns its spec handle. The
// first call is slot 0 (the default a step's VUs connect to); subsequent calls
// take slot indices in declaration order.
//
// For slot 0, the operator's standard params --driver.url/--driver.kind (env
// STROPPY_DRIVER_URL/STROPPY_DRIVER_KIND) override the declared url/kind; for
// later slots, STROPPY_DRIVER<N>_URL/STROPPY_DRIVER<N>_KIND apply (multi-driver,
// F2). opts carry driver-native config (URL, pool bounds — D2 class B/C/D); the
// URL opt seeds slot 0's standard-param default when the operator sets neither.
func (d *Def) Driver(name, kind string, opts ...DriverOpt) *DriverSpec {
	if name == "" {
		d.set.fail(fmt.Errorf("bench: driver slot %d has an empty name", len(d.drivers)))
		return &DriverSpec{name: "<empty>", kind: kind}
	}
	if _, dup := d.drvName[name]; dup {
		d.set.fail(fmt.Errorf("bench: driver slot %q declared twice", name))
		return &DriverSpec{name: name, kind: kind}
	}
	if kind == "" {
		kind = "pg"
	}
	spec := &DriverSpec{name: name, kind: kind, url: ""}
	for _, o := range opts {
		o(spec)
	}
	d.drivers = append(d.drivers, spec)
	d.drvName[name] = spec

	// Slot 0 honors the operator's standard driver.url/driver.kind params; an
	// empty standard value falls back to the spec's declared url/kind. Slots
	// beyond the first honor STROPPY_DRIVER<N>_URL / _KIND (multi-driver, F2).
	if len(d.drivers) == 1 {
		spec.url, spec.kind = d.resolveSlot0(spec.url, spec.kind)
	} else if d.run.getenv != nil {
		idx := len(d.drivers) - 1
		if u := d.run.getenv(fmt.Sprintf("STROPPY_DRIVER%d_URL", idx)); u != "" {
			spec.url = u
		}
		if k := d.run.getenv(fmt.Sprintf("STROPPY_DRIVER%d_KIND", idx)); k != "" {
			spec.kind = k
		}
	}
	if spec.kind == "" {
		spec.kind = "pg"
	}
	d.run.slots = append(d.run.slots, slotSpec{name: spec.name, kind: spec.kind, url: spec.url})
	return spec
}

// resolveSlot0 folds slot 0's standard-param overrides onto the declared
// defaults. The driver.url/driver.kind standard params default to empty, so an
// unset standard value (source = Default) means "use the declared default".
func (d *Def) resolveSlot0(declURL, declKind string) (url, kind string) {
	url, kind = declURL, declKind
	if u := d.run.stdDriverURL; u != nil {
		if v := u.Value(); v != "" {
			url = v
		}
	}
	if k := d.run.stdDriverKind; k != nil {
		if v := k.Value(); v != "" {
			kind = v
		}
	}
	return url, kind
}

// Queries registers one baked query-set and resolves it eagerly against the
// active driver kind. generic is the kind-neutral reference dialect; per-kind
// overrides arrive via opts ([PerKind]). Resolution (override -> per-kind ->
// generic) and parse happen now, so [QuerySet.File] is usable immediately
// inside Define (F1: no purity police, no IO enforcement). The author surfaces a
// resolution failure by returning it from Define.
func (d *Def) Queries(name string, generic []byte, opts ...QuerySetOpt) *QuerySet {
	bake := &BakedQuerySet{Name: name, Generic: generic}
	for _, o := range opts {
		o(bake)
	}
	if d.run.bakes == nil {
		d.run.bakes = make(map[string]*BakedQuerySet)
	}
	d.run.bakes[name] = bake
	qs := &QuerySet{name: name, run: d.run}
	// Eager resolution: parse + memoize now so File()/Section() are immediate.
	rs := d.run.resolveQuerySet(name)
	qs.file, qs.err = rs.file, rs.err
	return qs
}

// Step declares one DAG step: a named Handler plus its executor policy, edges,
// condition and driver slot. Chain policy/edges onto the returned *StepDef. The
// step is added to the declaration order (the basis for status reporting and
// build order); it enters the default "full" variant set automatically.
func (d *Def) Step(name string, h Handler) *StepDef {
	sd := Step(name, h)
	d.steps = append(d.steps, sd)
	if d.stepByName == nil {
		d.stepByName = make(map[string]*StepDef)
	}
	d.stepByName[name] = sd
	return sd
}

// Variant declares a named subgraph of the test. An empty steps list means "all
// declared steps" (D3b). At least one variant named "full" is required (D5); if
// no variant is declared at all, the SDK synthesizes "full" = all steps. The
// operator picks the active variant via the standard `variant` param.
func (d *Def) Variant(name string, steps ...*StepDef) {
	if name == "" {
		d.set.fail(fmt.Errorf("bench: variant has an empty name"))
		return
	}
	if _, dup := d.varByName[name]; dup {
		d.set.fail(fmt.Errorf("bench: variant %q declared twice", name))
		return
	}
	v := variantDef{name: name}
	if len(steps) > 0 {
		v.steps = make(map[string]bool, len(steps))
		for _, s := range steps {
			if s == nil {
				continue
			}
			v.steps[s.name] = true
		}
	}
	d.variants = append(d.variants, v)
	if d.varByName == nil {
		d.varByName = make(map[string]*variantDef)
	}
	d.varByName[name] = &d.variants[len(d.variants)-1]
}

// Histogram declares one histogram instrument, taggable by tx/query/table
// (D6). The returned handle is a forward reference: its underlying
// [metrics.MetricHandle] is assigned at phase-3 registration (after Freeze);
// calling [Histogram.Handle] before then panics. D6 wires the registration; for
// now authors declare instruments and the schema records them.
func (d *Def) Histogram(name string, opts ...MetricOpt) *Histogram {
	tags := collectTags(opts)
	h := &Histogram{name: name, tags: tags, mh: &metricsHandle{isHist: true}}
	d.instruments = append(d.instruments, &instrumentDecl{
		kind: kindHistogram, name: name, tags: tags, handle: h.mh,
	})
	return h
}

// Counter declares one counter instrument. See [Def.Histogram] for the
// forward-ref contract.
func (d *Def) Counter(name string, opts ...MetricOpt) *Counter {
	tags := collectTags(opts)
	c := &Counter{name: name, tags: tags, mh: &metricsHandle{}}
	d.instruments = append(d.instruments, &instrumentDecl{
		kind: kindCounter, name: name, tags: tags, handle: c.mh,
	})
	return c
}

func collectTags(opts []MetricOpt) []instrumentTag {
	var tags []instrumentTag
	for _, o := range opts {
		tags = append(tags, o()...)
	}
	return tags
}

// DriverSpec is the handle to one declared driver slot. The slot index is its
// position in declaration order (slot 0 first); [StepDef.Uses] targets a slot
// by name. D2 will add pin/derive config methods here.
type DriverSpec struct {
	name string
	kind string
	url  string
}

// Name reports the slot name.
func (s *DriverSpec) Name() string { return s.name }

// Kind reports the resolved driver kind (operator override honored for slot 0).
func (s *DriverSpec) Kind() string { return s.kind }

// URL reports the resolved connection URL (operator override honored for slot 0).
func (s *DriverSpec) URL() string { return s.url }

// DriverOpt reconfigures a driver slot at declaration (D2 class B/C/D config).
type DriverOpt func(*DriverSpec)

// WithURL sets the slot's declared connection URL — the default the operator's
// --driver.url standard param overrides for slot 0.
func WithURL(url string) DriverOpt {
	return func(s *DriverSpec) { s.url = url }
}

// PerKind adds a kind-specific baked source to a query-set, winning over the
// generic source for the matching driver kind (D3).
func PerKind(kind string, src []byte) QuerySetOpt {
	return func(b *BakedQuerySet) {
		if b.PerKind == nil {
			b.PerKind = make(map[string][]byte)
		}
		b.PerKind[kind] = src
	}
}

// QuerySetOpt reconfigures a query-set declaration.
type QuerySetOpt func(*BakedQuerySet)

// QuerySet is the handle to one declared, eagerly-resolved query-set. File
// returns the parsed corpus immediately (F1: usable inside Define).
type QuerySet struct {
	name string
	run  *Run
	file *sqlfile.File
	err  error
}

// File returns the parsed corpus and any resolution/parse error. Resolution is
// eager (done at [Def.Queries] time), so this is a cached read.
func (q *QuerySet) File() (*sqlfile.File, error) { return q.file, q.err }

// Section returns the queries in section name of the parsed corpus, or an error
// if resolution failed. A missing section yields an empty slice (D3: a missing
// section is valid -> skip, surfaced by introspection rather than a crash).
func (q *QuerySet) Section(name string) ([]*sqlfile.Query, error) {
	if q.err != nil {
		return nil, q.err
	}
	if q.file == nil {
		return nil, nil
	}
	return q.file.Section(name), nil
}

// MetricOpt adds tag columns to a declared instrument (D6).
type MetricOpt func() []instrumentTag

// Tag adds one tag column (e.g. tx, query, table) to an instrument. Multiple
// Tag opts fan the instrument out one handle per combination (D6).
func Tag(name string) MetricOpt {
	return func() []instrumentTag { return []instrumentTag{{name: name}} }
}

// Histogram is the forward-ref handle to a declared histogram instrument. The
// underlying metrics handle is assigned at phase-3 registration (after Freeze);
// Handle panics before then (D6 wires the assignment).
type Histogram struct {
	name string
	tags []instrumentTag
	mh   *metricsHandle
}

// Name reports the instrument name.
func (h *Histogram) Name() string { return h.name }

// Handle returns the underlying metrics handle for direct recording via
// [VU.M]. It panics until phase-3 registration assigns it (D6).
func (h *Histogram) Handle() metrics.MetricHandle {
	if h.mh == nil || !h.mh.assigned {
		panic("bench: histogram " + h.name + ": handle not assigned (phase-3 registration lands with D6)")
	}
	return h.mh.mh
}

// Counter is the forward-ref handle to a declared counter instrument. See
// [Histogram] for the forward-ref contract.
type Counter struct {
	name string
	tags []instrumentTag
	mh   *metricsHandle
}

// Name reports the instrument name.
func (c *Counter) Name() string { return c.name }

// Handle returns the underlying counter handle for direct recording via
// [VU.Inc]/[VU.Add]. It panics until phase-3 registration assigns it (D6).
func (c *Counter) Handle() metrics.CounterHandle {
	if c.mh == nil || !c.mh.assigned {
		panic("bench: counter " + c.name + ": handle not assigned (phase-3 registration lands with D6)")
	}
	return c.mh.ch
}
