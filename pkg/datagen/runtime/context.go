package runtime

import (
	"fmt"
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/datagen/cohort"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/lookup"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
	"github.com/stroppy-io/stroppy/pkg/datagen/stdlib"
)

// evalContext adapts a Runtime's per-row state to the expr.Context
// interface. A single evalContext is reused across rows: Runtime mutates
// scratch, indices, and active block cache between evaluations rather
// than allocating a fresh context each row.
//
// The flat runtime (no relationships) uses the fields scratch, rowIdx,
// and dicts. The relationship runtime additionally populates blocks,
// registry, iter, outerPop, and the entity/line/global indices.
type evalContext struct {
	scratch     map[string]any
	scratchKeys []string // ordered list of keys written in current row (for fast clear)
	dicts       map[string]*dgproto.Dict
	registry    *lookup.LookupRegistry
	cohorts     *cohort.Registry

	// cohortBucketKeys holds each schedule's default bucket_key Expr so
	// CohortDraw / CohortLive arms that omit a per-arm override can
	// still resolve one. Keys missing from the map indicate the
	// schedule has no default; the arm must carry its own bucket_key.
	cohortBucketKeys map[string]*dgproto.Expr

	// blocks is the cache of resolved BlockSlot values for the current
	// outer entity. It is refreshed at every outer-boundary transition
	// by the relationship runtime.
	blocks *blockCache

	// outerPop names the population projected onto the outer side of
	// the active relationship. Empty in flat mode.
	outerPop string

	// iterPop names the RelSource's own population (the inner side in
	// a relationship). Empty in flat mode.
	iterPop string

	// rowIdx is the single counter used by the flat runtime and is
	// reported for every RowIndex kind in that mode. In relationship
	// mode it mirrors the GLOBAL counter.
	rowIdx int64

	// entityIdx is the outer entity index `e` in relationship mode.
	entityIdx int64

	// lineIdx is the inner line index `i` in relationship mode.
	lineIdx int64

	// inRelationship switches RowIndex resolution between flat and
	// relationship semantics.
	inRelationship bool

	// rootSeed is the InsertSpec's seed; Draw composes it with attrPath,
	// streamID, and rowIdx through seed.Derive.
	rootSeed uint64

	// attrPath names the attr currently being evaluated. Runtime sets
	// this before calling into expr.Eval so StreamDraw / Choose mix
	// the attr identity into the per-draw seed.
	attrPath string

	// drawPRNG is lazily allocated and re-seeded on every Draw call so
	// StreamDraw / Choose avoid a fresh rand.New per sample.
	drawPRNG *seed.ReusablePRNG
}

// LookupCol resolves a ColRef by consulting the current row's scratch
// map, returning expr.ErrUnknownCol when the referenced attr has not yet
// been evaluated (for example, a forward reference or a DAG bug).
func (c *evalContext) LookupCol(name string) (any, error) {
	value, ok := c.scratch[name]
	if !ok {
		return nil, expr.ErrUnknownCol
	}

	return value, nil
}

// RowIndex returns the counter matching the requested kind. In flat
// mode every kind collapses onto rowIdx; in relationship mode ENTITY,
// LINE, and GLOBAL are distinct and UNSPECIFIED aliases GLOBAL.
func (c *evalContext) RowIndex(kind dgproto.RowIndex_Kind) int64 {
	if !c.inRelationship {
		return c.rowIdx
	}

	switch kind {
	case dgproto.RowIndex_ENTITY:
		return c.entityIdx
	case dgproto.RowIndex_LINE:
		return c.lineIdx
	case dgproto.RowIndex_GLOBAL, dgproto.RowIndex_UNSPECIFIED:
		return c.rowIdx
	default:
		return c.rowIdx
	}
}

// LookupDict returns the Dict identified by key from the InsertSpec's
// dicts map, or expr.ErrDictMissing when the key is absent.
func (c *evalContext) LookupDict(key string) (*dgproto.Dict, error) {
	dict, ok := c.dicts[key]
	if !ok {
		return nil, expr.ErrDictMissing
	}

	return dict, nil
}

// Call forwards to the package-internal stdlib dispatch. The runtime
// does not own or shadow the registry; stdlib owns its catalog.
func (c *evalContext) Call(name string, args []any) (any, error) {
	return stdlib.Call(name, args)
}

// BlockSlot returns the cached BlockSlot value for the current outer
// entity. The flat runtime has no Sides, so every call errors.
func (c *evalContext) BlockSlot(slot string) (any, error) {
	if c.blocks == nil {
		return nil, fmt.Errorf("%w: block slot %q outside relationship", expr.ErrBadExpr, slot)
	}

	return c.blocks.get(slot)
}

// Lookup routes a Lookup Expr: same-population reads resolve to the
// scratch of the current row (iter-side ColRef semantics), while
// sibling reads go through the LookupPop registry. A flat-mode context
// has no registry and reports ErrBadExpr unless the lookup targets the
// flat population itself (which would just be a row-scratch read).
func (c *evalContext) Lookup(popName, attrName string, entityIdx int64) (any, error) {
	if c.inRelationship && popName == c.iterPop {
		// Inner-side self-read: only the current row's scratch is
		// valid. A Lookup at a different entity index would require
		// the inner side to also be declared as a LookupPop, which is
		// not a pattern the plan requires.
		if entityIdx != c.entityIdx {
			return nil, fmt.Errorf(
				"%w: inner-side lookup at idx %d != current outer entity %d",
				expr.ErrBadExpr, entityIdx, c.entityIdx,
			)
		}

		value, ok := c.scratch[attrName]
		if !ok {
			return nil, expr.ErrUnknownCol
		}

		return value, nil
	}

	if c.registry == nil {
		return nil, fmt.Errorf("%w: no lookup registry for pop %q",
			expr.ErrBadExpr, popName)
	}

	return c.registry.Get(popName, attrName, entityIdx)
}

// Draw returns a PRNG seeded deterministically from (rootSeed,
// attrPath, streamID, rowIdx) via seed.Derive. The stream_id is
// hashed as the ASCII bytes "s" + decimal(rowIdx) to avoid string
// allocation — see seed.fnv1a64 for the in-line hashing.
func (c *evalContext) Draw(streamID uint32, attrPath string, rowIdx int64) *rand.Rand {
	if c.drawPRNG == nil {
		c.drawPRNG = seed.NewReusablePRNG()
	}

	key := c.deriveDraw(streamID, attrPath, rowIdx)
	c.drawPRNG.Seed(key)

	return c.drawPRNG.Rand()
}

// DrawKey returns a PRNG seeded directly from key, reusing the same per-runtime
// source as Draw. It is intentionally not part of expr.Context; hot paths that
// already derived a substream key can use it via an optional interface.
func (c *evalContext) DrawKey(key uint64) *rand.Rand {
	if c.drawPRNG == nil {
		c.drawPRNG = seed.NewReusablePRNG()
	}

	c.drawPRNG.Seed(key)

	return c.drawPRNG.Rand()
}

// deriveDraw computes the seed for a StreamDraw call without allocating strings.
// It hashes (attrPath, "s" + decimal(streamID), decimal(rowIdx)) using the
// same formula as seed.Derive, but avoids string allocation by hashing each
// element's bytes directly.
func (c *evalContext) deriveDraw(streamID uint32, attrPath string, rowIdx int64) uint64 {
	const prefix = 's'

	var h uint64 = 0x9E3779B97F4A7C15 // splitmix64 gamma

	// Hash attrPath (with "/" prefix).
	h ^= 0x9E3779B97F4A7C15 ^ '/' // fnv1a offset ^ '/'
	h *= 0x9E3779B97F4A7C15
	for i := 0; i < len(attrPath); i++ {
		h ^= uint64(attrPath[i])
		h *= 0x9E3779B97F4A7C15
	}

	// Hash "s" prefix.
	h ^= prefix
	h *= 0x9E3779B97F4A7C15

	// Hash streamID as decimal bytes.
	for d := uint32(1); d <= streamID; d *= 10 {
		h ^= uint64('0' + byte(streamID/d%10))
		h *= 0x9E3779B97F4A7C15
	}

	// Hash rowIdx as decimal bytes (with "-" sign if negative).
	if rowIdx < 0 {
		h ^= '-'
		h *= 0x9E3779B97F4A7C15
		rowIdx = -rowIdx
	}
	for d := int64(1); d <= rowIdx; d *= 10 {
		h ^= uint64('0' + byte(rowIdx/d%10))
		h *= 0x9E3779B97F4A7C15
	}

	return seed.SplitMix64(h)
}

// AttrPath returns the attr currently being evaluated. Empty when no
// attr is active (e.g. a test harness that bypasses Runtime).
func (c *evalContext) AttrPath() string {
	return c.attrPath
}

// CohortDraw forwards to the runtime's cohort registry. A flat spec
// that declares no cohorts reports ErrBadCohort.
func (c *evalContext) CohortDraw(name string, bucketKey, slot int64) (int64, error) {
	if c.cohorts == nil {
		return 0, fmt.Errorf("%w: no cohort registry", expr.ErrBadCohort)
	}

	return c.cohorts.Draw(name, bucketKey, slot)
}

// CohortLive forwards to the runtime's cohort registry. A flat spec
// that declares no cohorts reports ErrBadCohort.
func (c *evalContext) CohortLive(name string, bucketKey int64) (bool, error) {
	if c.cohorts == nil {
		return false, fmt.Errorf("%w: no cohort registry", expr.ErrBadCohort)
	}

	return c.cohorts.Live(name, bucketKey)
}

// CohortBucketKey returns the default bucket_key Expr declared on the
// named schedule. Absent schedules and schedules without a default
// return nil; callers fall back to the per-arm bucket_key.
func (c *evalContext) CohortBucketKey(name string) *dgproto.Expr {
	if c.cohortBucketKeys == nil {
		return nil
	}

	return c.cohortBucketKeys[name]
}
