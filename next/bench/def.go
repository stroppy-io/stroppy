package bench

import (
	"fmt"
	"strings"

	"github.com/stroppy-io/stroppy/next/driver"
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
// registered with the metrics.Registry at phase 3 by [assignInstruments] (D6).
// Until that runs the handle's [Histogram.Handle]/[Histogram.For]/[Counter...]
// methods panic, since the underlying MetricHandle is not yet assigned.
type instrumentDecl struct {
	kind   instrumentKind
	name   string
	tags   []instrumentTag
	handle *metricsHandle // the forward-ref handle assigned at phase 3
}

type instrumentKind uint8

const (
	kindHistogram instrumentKind = iota
	kindCounter
)

// instrumentTag is one tag column of an author instrument (D6 tag columns). A
// tag with a declared value space (Values non-empty) fans the instrument out to
// one handle per value at phase-3 registration; a tag with no values declares
// the column only (no fan-out). Stage-1 uses the first tag carrying values for
// fan-out; multi-tag fan-out is a stage-2 concern.
type instrumentTag struct {
	name   string
	values []string
}

// metricsHandle is the shared forward-ref state of a Histogram/Counter. The
// underlying metrics handle is unassigned until phase-3 registration
// ([assignInstruments]) runs. A tagged instrument (one declared with at least
// one tag value) holds a per-value map; an untagged one holds a single handle.
type metricsHandle struct {
	assigned bool
	isHist   bool
	tagged   bool                          // true when a tag column has values
	single   metrics.MetricHandle          // untagged histogram
	singleC  metrics.CounterHandle         // untagged counter
	byValue  map[string]metrics.MetricHandle  // tagged histogram (keyed by tag value)
	byValueC map[string]metrics.CounterHandle // tagged counter
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
// later slots the name-based env STROPPY_DRIVER<NAME>_URL/_KIND (upper-cased
// slot name) applies first, falling back to the index form
// STROPPY_DRIVER<N>_URL/_KIND (multi-driver, F2). opts carry driver-native
// config: [WithURL], acquisition ([Shared]/[PerVU]), pool bounds ([Pool],
// [Pin], [Derive]) and native knobs ([NativeKV]) — D2 class B/C/D. The URL opt
// seeds slot 0's standard-param default when the operator sets neither.
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
	spec := &DriverSpec{name: name, kind: kind}
	for _, o := range opts {
		o(spec)
	}
	// Inherit the URL set by [WithURL] into the working value the env override
	// below folds onto, then back into the resolved Spec at the end.
	if spec.url == "" {
		spec.url = spec.spec.URL
	}
	d.drivers = append(d.drivers, spec)
	d.drvName[name] = spec

	// Slot 0 honors the operator's standard driver.url/driver.kind params; an
	// empty standard value falls back to the spec's declared url/kind. Slots
	// beyond the first honor the name-based env (upper-cased slot name) first,
	// then the index form (multi-driver, F2).
	if len(d.drivers) == 1 {
		spec.url, spec.kind = d.resolveSlot0(spec.url, spec.kind)
	} else {
		idx := len(d.drivers) - 1
		spec.url, spec.kind = d.resolveExtraSlot(name, idx, spec.url, spec.kind)
	}
	if spec.kind == "" {
		spec.kind = "pg"
	}
	spec.spec.URL = spec.url
	d.run.slots = append(d.run.slots, slotSpec{name: spec.name, kind: spec.kind, spec: spec.spec})
	return spec
}

// resolveExtraSlot folds the name-based then index-based env overrides for a
// slot beyond the first onto its declared defaults (multi-driver, F2).
func (d *Def) resolveExtraSlot(name string, idx int, declURL, declKind string) (url, kind string) {
	url, kind = declURL, declKind
	if d.run.getenv == nil {
		return url, kind
	}
	envName := strings.ToUpper(name)
	if u := d.run.getenv("STROPPY_DRIVER"+envName+"_URL"); u != "" {
		url = u
	} else if u := d.run.getenv(fmt.Sprintf("STROPPY_DRIVER%d_URL", idx)); u != "" {
		url = u
	}
	if k := d.run.getenv("STROPPY_DRIVER" + envName + "_KIND"); k != "" {
		kind = k
	} else if k := d.run.getenv(fmt.Sprintf("STROPPY_DRIVER%d_KIND", idx)); k != "" {
		kind = k
	}
	return url, kind
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
// (D6). The returned handle is a forward reference: its underlying handles are
// assigned at phase-3 registration by [assignInstruments] (before Freeze);
// calling [Histogram.Handle] or [Histogram.For] before then panics.
//
// A tag declared with values — Tag("tx", "new_order", "payment", ...) — fans
// the instrument out to one handle per value; the author resolves the right one
// per record via [Histogram.For]. A tag declared without values records the
// column only (no fan-out); the instrument has a single handle ([Histogram.Handle]).
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

// assignInstruments is the phase-3 registration seam (D6): it walks every
// author-declared instrument and registers it with the shared metrics.Registry,
// resolving the forward-ref handles to real MetricHandles/CounterHandles. It
// runs BEFORE per-step built-in registration (so author instruments join the 5
// built-ins in the same Registry) and BEFORE Freeze (so shards minted later see
// the full set). The Registry must not yet be frozen.
//
// For a tag column with declared values, one instrument is registered per value
// (fan-out, D6); the value rides on the matching Instrument field (Tag name
// "tx" -> Instrument.Tx, "table" -> Instrument.Table, otherwise Tx). An
// untagged instrument registers once.
func assignInstruments(d *Def, reg *metrics.Registry) {
	for _, decl := range d.instruments {
		decl.handle.assign(reg, decl)
	}
}

// assign registers decl's instrument(s) into reg and resolves the forward-ref
// handle. A tag column with values fans out one instrument per declared value.
func (mh *metricsHandle) assign(reg *metrics.Registry, decl *instrumentDecl) {
	tagName, values := fanOutAxis(decl.tags)
	if len(values) == 0 {
		inst := metrics.Instrument{Name: decl.name}
		if mh.isHist {
			mh.single = reg.Histogram(inst)
		} else {
			mh.singleC = reg.Counter(inst)
		}
		mh.assigned = true
		return
	}
	mh.tagged = true
	if mh.isHist {
		mh.byValue = make(map[string]metrics.MetricHandle, len(values))
	} else {
		mh.byValueC = make(map[string]metrics.CounterHandle, len(values))
	}
	for _, v := range values {
		inst := metrics.Instrument{Name: decl.name}
		applyTag(&inst, tagName, v)
		if mh.isHist {
			mh.byValue[v] = reg.Histogram(inst)
		} else {
			mh.byValueC[v] = reg.Counter(inst)
		}
	}
	mh.assigned = true
}

// fanOutAxis picks the tag column driving fan-out for decl: the first tag with
// a non-empty value space. Stage-1 supports a single fan-out axis; later tags
// (if any) are recorded but do not multiply the handle set. Returns ("", nil)
// when no tag carries values — the instrument registers once.
func fanOutAxis(tags []instrumentTag) (name string, values []string) {
	for _, t := range tags {
		if len(t.values) > 0 {
			return t.name, t.values
		}
	}
	return "", nil
}

// applyTag writes value into the Instrument field matching tagName ("tx" -> Tx,
// "table" -> Table). Any other tag name maps to Tx — the field that backs the
// per-tx mix today. Stage-1 simplification: extend Instrument with more tag
// fields when a workload needs them.
func applyTag(inst *metrics.Instrument, tagName, value string) {
	switch tagName {
	case "table":
		inst.Table = value
	default:
		inst.Tx = value
	}
}

// DriverSpec is the handle to one declared driver slot. The slot index is its
// position in declaration order (slot 0 first); [StepDef.Uses] targets a slot
// by name. It carries the resolved [driver.Spec]: acquisition mode (Shared vs
// PerVU), pool bounds, and the opaque Native map a per-dbdrv accessor reads.
type DriverSpec struct {
	name string
	kind string
	url  string
	spec driver.Spec
}

// Name reports the slot name.
func (s *DriverSpec) Name() string { return s.name }

// Kind reports the resolved driver kind (operator override honored for slot 0).
func (s *DriverSpec) Kind() string { return s.kind }

// URL reports the resolved connection URL (operator override honored for slot 0).
func (s *DriverSpec) URL() string { return s.url }

// Mode reports the resolved acquisition mode (PerVU default, Shared when
// [Shared] was declared).
func (s *DriverSpec) Mode() driver.Acquisition { return s.spec.Mode }

// Spec reports the resolved driver configuration: URL, pool bounds, Native
// knobs and per-field provenance (D2 introspection).
func (s *DriverSpec) Spec() driver.Spec { return s.spec }

// DriverOpt reconfigures a driver slot at declaration (D2 class B/C/D config).
type DriverOpt func(*DriverSpec)

// WithURL sets the slot's declared connection URL — the default the operator's
// --driver.url standard param overrides for slot 0.
func WithURL(url string) DriverOpt {
	return func(s *DriverSpec) { s.url = url; s.spec.URL = url }
}

// Shared selects the shared-pool acquisition mode: one connection pool across
// every VU of the slot's step; a VU borrows per use and returns on Close (D2/
// F2). The default is [PerVU]; [Shared] is for non-measured slots. Pool bounds
// ([Pool], [Pin], [Derive]) become meaningful under Shared.
func Shared() DriverOpt {
	return func(s *DriverSpec) { s.spec.Mode = driver.Shared; s.spec.SetSource("mode", driver.FieldPinned) }
}

// PerVU explicitly selects the pinned-conn acquisition mode (the default): one
// dedicated connection per VU, no pool contention (RFC 0001 §10).
func PerVU() DriverOpt {
	return func(s *DriverSpec) { s.spec.Mode = driver.PerVU; s.spec.SetSource("mode", driver.FieldPinned) }
}

// PoolBounds sets the connection-pool bounds (Shared only; a PerVU slot ignores
// them). min/max are the pool's target size and hard cap. Recorded as a soft
// default — prefer [Pin] for a hard requirement or [Derive] to compute the size
// from a resolved param.
func PoolBounds(min, max int) DriverOpt {
	return func(s *DriverSpec) {
		s.spec.MinConns = int32(min)
		s.spec.MaxConns = int32(max)
	}
}

// Pin sets field to a hard requirement that wins over an operator override (D2
// class C). Recognized fields are [PoolMin] and [PoolMax]; e.g. a single-
// connection test pins Pin(PoolMax, 1).
func Pin(field string, val int) DriverOpt {
	return func(s *DriverSpec) {
		switch field {
		case PoolMin:
			s.spec.MinConns = int32(val)
			s.spec.SetSource(PoolMin, driver.FieldPinned)
		case PoolMax:
			s.spec.MaxConns = int32(val)
			s.spec.SetSource(PoolMax, driver.FieldPinned)
		}
	}
}

// Derive computes field from other resolved params at declaration time (D2 class
// C), e.g. Derive(PoolMax, func() int { return vus.Value() }) for pool=vus.
// Recognized fields are [PoolMin] and [PoolMax].
func Derive(field string, fn func() int) DriverOpt {
	return func(s *DriverSpec) {
		val := int32(fn())
		switch field {
		case PoolMin:
			s.spec.MinConns = val
			s.spec.SetSource(PoolMin, driver.FieldDerived)
		case PoolMax:
			s.spec.MaxConns = val
			s.spec.SetSource(PoolMax, driver.FieldDerived)
		}
	}
}

// NativeKV adds one driver-specific advanced knob (D2 class B) to the slot's
// opaque Native map; a per-dbdrv typed accessor (pg.Native) reads it. Repeat to
// set several. Auth/TLS are not native knobs — they fold into the URL.
func NativeKV(key string, val any) DriverOpt {
	return func(s *DriverSpec) { s.spec.SetNative(key, val) }
}

// Pool-field constants for [Pin]/[Derive].
const (
	PoolMin = "pool_min"
	PoolMax = "pool_max"
)

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

// Tag adds one tag column (e.g. tx, query, table) to an instrument. Optional
// values declare the tag's enum space: when non-empty, the instrument fans out
// to one handle per value (D6 tag column = fan-out) and the author resolves the
// per-record handle via [Histogram.For]/[Counter.For]. With no values the
// column is declared only and the instrument has a single handle.
func Tag(name string, values ...string) MetricOpt {
	return func() []instrumentTag { return []instrumentTag{{name: name, values: values}} }
}

// Histogram is the forward-ref handle to a declared histogram instrument. The
// underlying handles are assigned at phase-3 registration by [assignInstruments]
// (before Freeze); Handle/For panic until then.
type Histogram struct {
	name string
	tags []instrumentTag
	mh   *metricsHandle
}

// Name reports the instrument name.
func (h *Histogram) Name() string { return h.name }

// Handle returns the single metrics handle for an untagged histogram, for
// direct recording via [VU.M]. It panics until phase-3 registration assigns it,
// and panics for a tagged histogram (call [Histogram.For] with the tag value
// instead).
func (h *Histogram) Handle() metrics.MetricHandle {
	if h.mh == nil || !h.mh.assigned {
		panic("bench: histogram " + h.name + ": handle not assigned (phase-3 registration pending)")
	}
	if h.mh.tagged {
		panic("bench: histogram " + h.name + ": tagged instrument; use For(value)")
	}
	return h.mh.single
}

// For returns the metrics handle for one tag value of a tagged histogram. For
// an untagged histogram it returns the single handle regardless of value, so
// the same call site works before and after adding a Tag. Panics until phase-3
// registration runs, and panics for an undeclared value (a programming bug —
// the tag's enum space is fixed at declaration).
func (h *Histogram) For(value string) metrics.MetricHandle {
	if h.mh == nil || !h.mh.assigned {
		panic("bench: histogram " + h.name + ": handle not assigned (phase-3 registration pending)")
	}
	if !h.mh.tagged {
		return h.mh.single
	}
	m, ok := h.mh.byValue[value]
	if !ok {
		panic("bench: histogram " + h.name + ": tag value " + value + " not declared")
	}
	return m
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

// Handle returns the single counter handle for an untagged counter. See
// [Histogram.Handle] for the panic contract; use [Counter.For] for a tagged
// counter.
func (c *Counter) Handle() metrics.CounterHandle {
	if c.mh == nil || !c.mh.assigned {
		panic("bench: counter " + c.name + ": handle not assigned (phase-3 registration pending)")
	}
	if c.mh.tagged {
		panic("bench: counter " + c.name + ": tagged instrument; use For(value)")
	}
	return c.mh.singleC
}

// For returns the counter handle for one tag value of a tagged counter. See
// [Histogram.For] for the untagged/panic contract.
func (c *Counter) For(value string) metrics.CounterHandle {
	if c.mh == nil || !c.mh.assigned {
		panic("bench: counter " + c.name + ": handle not assigned (phase-3 registration pending)")
	}
	if !c.mh.tagged {
		return c.mh.singleC
	}
	ch, ok := c.mh.byValueC[value]
	if !ok {
		panic("bench: counter " + c.name + ": tag value " + value + " not declared")
	}
	return ch
}
