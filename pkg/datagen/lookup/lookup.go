// Package lookup holds the cross-population read path for the datagen
// runtime. A LookupRegistry compiles every LookupPop declared on an
// enclosing RelSource, evaluates their attr DAGs lazily per entity
// index, and caches recent rows in a bounded LRU. The same registry
// answers reads for the outer side of a relationship, which must also
// be declared as a LookupPop so that its full attr DAG is available
// when the inner side iterates.
package lookup

import (
	"container/list"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"

	"github.com/stroppy-io/stroppy/pkg/datagen/compile"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

// DefaultCacheSize caps each LookupPop's LRU unless overridden by the
// caller or the STROPPY_LOOKUP_CACHE_SIZE env var.
const DefaultCacheSize = 10_000

// cacheSizeEnv is the env var that overrides the default LRU cap.
const cacheSizeEnv = "STROPPY_LOOKUP_CACHE_SIZE"

// ErrUnknownPop is returned when a Lookup or caller names a population
// the registry does not host.
var ErrUnknownPop = errors.New("lookup: unknown target population")

// ErrUnknownAttr is returned when a Lookup names an attr the target
// LookupPop does not declare.
var ErrUnknownAttr = errors.New("lookup: unknown attr in target population")

// ErrOutOfRange is returned when the resolved entity index is outside
// the target LookupPop's [0, size) domain. Callers that want modulo
// wrap must apply it explicitly before calling Get.
var ErrOutOfRange = errors.New("lookup: entity index out of range")

// ErrInvalidPop is returned when a LookupPop is missing its population,
// has non-positive size, or has no attrs.
var ErrInvalidPop = errors.New("lookup: invalid LookupPop")

// ErrDuplicatePop is returned when two LookupPops share a name.
var ErrDuplicatePop = errors.New("lookup: duplicate LookupPop name")

// ErrCycle is returned when resolving a Lookup recurses into a
// population currently being resolved.
var ErrCycle = errors.New("lookup: resolution cycle")

// pop holds one compiled LookupPop and its LRU cache.
type pop struct {
	name  string
	size  int64
	dag   *compile.DAG
	cache *rowCache
}

// rowCache is a bounded LRU of already-evaluated rows keyed by entity
// index. A row is a map from attr name to value (nil meaning a hit from
// a non-present attr is impossible here — attrs always produce a value,
// even if that value is nil via Null). We store the full row so that
// repeated attr reads at the same index share one evaluation.
type rowCache struct {
	cap   int
	order *list.List
	index map[int64]*list.Element
}

// cacheEntry binds an entity index to its attr row.
type cacheEntry struct {
	idx int64
	row map[string]any
}

// LookupRegistry routes Lookup reads to the right compiled LookupPop.
// It owns one bounded LRU per population. A single registry is
// single-owner: its caches and inFlight set are not guarded. Parallel
// workers must each get their own registry via CloneRegistry — runtime
// clones do so unconditionally.
type LookupRegistry struct {
	pops     map[string]*pop
	dicts    map[string]*dgproto.Dict
	inFlight map[string]struct{}
	rootSeed uint64
}

// NewLookupRegistry compiles the given LookupPops and returns a ready
// registry. cacheSize, if zero or negative, is resolved from the
// STROPPY_LOOKUP_CACHE_SIZE env var, else from DefaultCacheSize.
func NewLookupRegistry(
	lookupPops []*dgproto.LookupPop,
	dicts map[string]*dgproto.Dict,
	cacheSize int,
) (*LookupRegistry, error) {
	effective := resolveCacheSize(cacheSize)

	reg := &LookupRegistry{
		pops:     make(map[string]*pop, len(lookupPops)),
		dicts:    dicts,
		inFlight: make(map[string]struct{}),
	}

	for i, lp := range lookupPops {
		if lp == nil {
			return nil, fmt.Errorf("%w: nil LookupPop at %d", ErrInvalidPop, i)
		}

		compiled, err := compilePop(lp, effective)
		if err != nil {
			return nil, err
		}

		if _, dup := reg.pops[compiled.name]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicatePop, compiled.name)
		}

		reg.pops[compiled.name] = compiled
	}

	return reg, nil
}

// CloneRegistry returns an independent registry that shares the read-only
// DAG, population metadata, dict map, and root seed with the receiver,
// but owns fresh per-pop caches and a fresh inFlight set. The original
// registry is unaffected.
//
// Purpose: give every parallel worker its own cache/inFlight state so
// writes through the LRU do not race with sibling workers. Cache
// capacity is preserved per-clone — each clone's LRU is the same size
// as the source's, not a fraction of it.
func (r *LookupRegistry) CloneRegistry() *LookupRegistry {
	clone := &LookupRegistry{
		pops:     make(map[string]*pop, len(r.pops)),
		dicts:    r.dicts, // read-only after NewLookupRegistry
		inFlight: make(map[string]struct{}),
		rootSeed: r.rootSeed,
	}

	for name, src := range r.pops {
		clone.pops[name] = &pop{
			name:  src.name,
			size:  src.size,
			dag:   src.dag, // DAG is read-only after compile
			cache: newRowCache(src.cache.cap),
		}
	}

	return clone
}

// SetRootSeed installs the InsertSpec seed so the registry can forward
// it to the Draw(...) hook that LookupPop attrs reach for when they
// contain StreamDraw nodes. The runtime calls this once at Runtime
// construction, before any row is emitted.
func (r *LookupRegistry) SetRootSeed(rootSeed uint64) {
	r.rootSeed = rootSeed
}

// Has reports whether the registry hosts the named population.
func (r *LookupRegistry) Has(popName string) bool {
	_, ok := r.pops[popName]

	return ok
}

// Size returns the declared size of the named LookupPop.
func (r *LookupRegistry) Size(popName string) (int64, error) {
	population, ok := r.pops[popName]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownPop, popName)
	}

	return population.size, nil
}

// Get returns the value of attrName for the given entity index within
// popName. Rows are memoized per index in an LRU; a miss evaluates the
// target pop's full attr DAG at that index and caches it.
func (r *LookupRegistry) Get(popName, attrName string, entityIdx int64) (any, error) {
	population, ok := r.pops[popName]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownPop, popName)
	}

	if entityIdx < 0 || entityIdx >= population.size {
		return nil, fmt.Errorf("%w: %d not in [0, %d)", ErrOutOfRange, entityIdx, population.size)
	}

	if _, hasAttr := population.dag.Index[attrName]; !hasAttr {
		return nil, fmt.Errorf("%w: %q.%q", ErrUnknownAttr, popName, attrName)
	}

	row, err := r.rowAt(population, entityIdx)
	if err != nil {
		return nil, err
	}

	return row[attrName], nil
}

// rowAt returns the memoized attr row for (population, idx), evaluating the
// DAG on a miss. The row is inserted into the LRU on miss and promoted
// on hit.
func (r *LookupRegistry) rowAt(population *pop, idx int64) (map[string]any, error) {
	if row, hit := population.cache.get(idx); hit {
		return row, nil
	}

	if _, recursing := r.inFlight[population.name]; recursing {
		return nil, fmt.Errorf("%w: %q", ErrCycle, population.name)
	}

	r.inFlight[population.name] = struct{}{}
	defer delete(r.inFlight, population.name)

	row, err := r.evalRow(population, idx)
	if err != nil {
		return nil, err
	}

	population.cache.put(idx, row)

	return row, nil
}

// evalRow runs the compiled DAG of population at entity index idx and
// returns the attr-name → value map.
func (r *LookupRegistry) evalRow(population *pop, idx int64) (map[string]any, error) {
	scratch := make(map[string]any, len(population.dag.Order))
	ctx := &popCtx{
		reg:       r,
		scratch:   scratch,
		entityIdx: idx,
		dicts:     r.dicts,
		popName:   population.name,
	}

	for _, attr := range population.dag.Order {
		name := attr.GetName()
		ctx.attrPath = population.name + "/" + name

		value, err := expr.Eval(ctx, attr.GetExpr())
		if err != nil {
			return nil, fmt.Errorf("lookup: pop %q attr %q at entity %d: %w",
				population.name, name, idx, err)
		}

		scratch[name] = value
	}

	return scratch, nil
}

// compilePop validates a LookupPop and wraps it with a fresh cache.
func compilePop(lp *dgproto.LookupPop, cacheSize int) (*pop, error) {
	population := lp.GetPopulation()
	if population == nil {
		return nil, fmt.Errorf("%w: missing population", ErrInvalidPop)
	}

	name := population.GetName()
	if name == "" {
		return nil, fmt.Errorf("%w: empty population name", ErrInvalidPop)
	}

	size := population.GetSize()
	if size <= 0 {
		return nil, fmt.Errorf("%w: population %q size %d", ErrInvalidPop, name, size)
	}

	attrs := lp.GetAttrs()
	if len(attrs) == 0 {
		return nil, fmt.Errorf("%w: population %q has no attrs", ErrInvalidPop, name)
	}

	dag, err := compile.Build(attrs)
	if err != nil {
		return nil, fmt.Errorf("lookup: compile %q: %w", name, err)
	}

	return &pop{
		name:  name,
		size:  size,
		dag:   dag,
		cache: newRowCache(cacheSize),
	}, nil
}

// resolveCacheSize picks the effective LRU cap from the explicit arg,
// env override, and default. Explicit positive values win.
func resolveCacheSize(explicit int) int {
	if explicit > 0 {
		return explicit
	}

	if raw := os.Getenv(cacheSizeEnv); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
	}

	return DefaultCacheSize
}

// newRowCache returns a bounded LRU with the requested capacity.
func newRowCache(capacity int) *rowCache {
	if capacity < 1 {
		capacity = 1
	}

	return &rowCache{
		cap:   capacity,
		order: list.New(),
		index: make(map[int64]*list.Element, capacity),
	}
}

// get promotes and returns the cached row at idx, or reports a miss.
func (c *rowCache) get(idx int64) (map[string]any, bool) {
	elem, ok := c.index[idx]
	if !ok {
		return nil, false
	}

	c.order.MoveToFront(elem)

	entry, _ := elem.Value.(*cacheEntry)

	return entry.row, true
}

// put inserts (idx, row) at the MRU end, evicting the LRU entry if the
// cap is already reached. It is a no-op if idx is already present.
func (c *rowCache) put(idx int64, row map[string]any) {
	if _, ok := c.index[idx]; ok {
		return
	}

	if c.order.Len() >= c.cap {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)

			entry, _ := oldest.Value.(*cacheEntry)
			delete(c.index, entry.idx)
		}
	}

	elem := c.order.PushFront(&cacheEntry{idx: idx, row: row})
	c.index[idx] = elem
}

// Len returns the current number of entries in the cache. Test-only.
func (c *rowCache) Len() int {
	return c.order.Len()
}

// popCtx adapts a pop's DAG evaluation to the expr.Context interface.
// It resolves ColRefs in the scratch, RowIndex to the entity index,
// dicts via the registry, Calls via stdlib, and Lookups recursively
// through the registry. BlockRef is not defined in LookupPop scope —
// BlockSlots belong to Sides, not pure populations — so BlockRef
// returns a type error.
type popCtx struct {
	reg       *LookupRegistry
	scratch   map[string]any
	entityIdx int64
	dicts     map[string]*dgproto.Dict
	popName   string
	attrPath  string
}

// LookupCol resolves a ColRef within the LookupPop's own scratch.
func (c *popCtx) LookupCol(name string) (any, error) {
	value, ok := c.scratch[name]
	if !ok {
		return nil, expr.ErrUnknownCol
	}

	return value, nil
}

// RowIndex returns the entity index for the LookupPop row being
// computed. LookupPops have no inner iteration, so every kind (ENTITY,
// LINE, GLOBAL, UNSPECIFIED) collapses onto the same counter.
func (c *popCtx) RowIndex(_ dgproto.RowIndex_Kind) int64 {
	return c.entityIdx
}

// LookupDict proxies to the enclosing InsertSpec's dict map.
func (c *popCtx) LookupDict(key string) (*dgproto.Dict, error) {
	dict, ok := c.dicts[key]
	if !ok {
		return nil, expr.ErrDictMissing
	}

	return dict, nil
}

// Call forwards to stdlib. LookupPop attrs may use any registered
// function.
func (c *popCtx) Call(name string, args []any) (any, error) {
	return stdlib.Call(name, args)
}

// BlockSlot is undefined in LookupPop scope; BlockSlots live on Sides.
func (c *popCtx) BlockSlot(slot string) (any, error) {
	return nil, fmt.Errorf("%w: BlockRef %q not available in LookupPop scope",
		expr.ErrBadExpr, slot)
}

// Lookup resolves transitively through the same registry.
func (c *popCtx) Lookup(popName, attrName string, entityIdx int64) (any, error) {
	return c.reg.Get(popName, attrName, entityIdx)
}

// Draw returns a PRNG for StreamDraw / Choose nodes inside a LookupPop
// attr. It uses the registry's rootSeed and the same Derive formula as
// the flat runtime, ensuring that a LookupPop attr that itself carries
// a random draw is still seekable.
func (c *popCtx) Draw(streamID uint32, attrPath string, rowIdx int64) *rand.Rand {
	key := seed.Derive(
		c.reg.rootSeed,
		attrPath,
		"s"+strconv.FormatUint(uint64(streamID), 10),
		strconv.FormatInt(rowIdx, 10),
	)

	return seed.PRNG(key)
}

// AttrPath returns the pop-qualified attr path currently under
// evaluation.
func (c *popCtx) AttrPath() string {
	return c.attrPath
}

// CohortDraw is undefined in LookupPop scope: pure sibling populations
// do not reach into cohort schedules. Callers that need cohort draws
// must express them on the owning RelSource, not on a lookup target.
func (c *popCtx) CohortDraw(name string, _, _ int64) (int64, error) {
	return 0, fmt.Errorf("%w: cohort %q not available in LookupPop scope",
		expr.ErrBadCohort, name)
}

// CohortLive is undefined in LookupPop scope for the same reason as
// CohortDraw.
func (c *popCtx) CohortLive(name string, _ int64) (bool, error) {
	return false, fmt.Errorf("%w: cohort %q not available in LookupPop scope",
		expr.ErrBadCohort, name)
}

// CohortBucketKey returns nil in LookupPop scope; the caller will then
// surface a BadCohort error when the arm has no per-arm bucket_key.
func (c *popCtx) CohortBucketKey(string) *dgproto.Expr {
	return nil
}
