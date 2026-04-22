package cohort

import (
	"container/list"
	"fmt"
	"math/rand/v2"
	"strconv"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// DefaultCacheSize caps each schedule's LRU of materialized bucket
// slot lists unless overridden at New() time.
const DefaultCacheSize = 10_000

// schedule is the compiled form of a dgproto.Cohort. It keeps only the
// fields needed at draw time; bucket_key is owned by the evaluator,
// because schedules are pure functions of (seed, bucket_key, slot).
type schedule struct {
	name             string
	cohortSize       int64
	entityMin        int64
	entityMax        int64
	span             int64
	activeEvery      int64
	persistenceMod   int64
	persistenceRatio float32
	seedSalt         uint64
	cache            *slotCache
}

// Registry answers Draw/Live queries for a set of compiled Cohort
// schedules. It is not safe for concurrent use; parallel workers build
// their own Registry from the same protos.
type Registry struct {
	schedules map[string]*schedule
	rootSeed  uint64
	cacheSize int
}

// New compiles the given Cohort protos into a Registry seeded by
// rootSeed. cacheSize, if zero or negative, falls back to
// DefaultCacheSize. Returns an error on duplicate names, invalid entity
// ranges, cohort_size > span + 1, or persistence ratios outside [0, 1].
func New(cohorts []*dgproto.Cohort, rootSeed uint64, cacheSize int) (*Registry, error) {
	if cacheSize <= 0 {
		cacheSize = DefaultCacheSize
	}

	reg := &Registry{
		schedules: make(map[string]*schedule, len(cohorts)),
		rootSeed:  rootSeed,
		cacheSize: cacheSize,
	}

	for i, c := range cohorts {
		if c == nil {
			return nil, fmt.Errorf("%w: nil Cohort at %d", ErrInvalidCohort, i)
		}

		compiled, err := compileSchedule(c, cacheSize)
		if err != nil {
			return nil, err
		}

		if _, dup := reg.schedules[compiled.name]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateCohort, compiled.name)
		}

		reg.schedules[compiled.name] = compiled
	}

	return reg, nil
}

// Has reports whether the registry hosts a schedule by the given name.
func (r *Registry) Has(name string) bool {
	_, ok := r.schedules[name]

	return ok
}

// Draw returns the entity ID at `slot` within the named cohort's
// schedule at bucketKey. Returns ErrUnknownCohort for an unknown name,
// ErrSlotRange if slot is not in [0, cohort_size).
func (r *Registry) Draw(name string, bucketKey, slot int64) (int64, error) {
	sched, ok := r.schedules[name]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownCohort, name)
	}

	if slot < 0 || slot >= sched.cohortSize {
		return 0, fmt.Errorf("%w: slot %d cohort_size %d",
			ErrSlotRange, slot, sched.cohortSize)
	}

	slots := r.slotsFor(sched, bucketKey)

	return slots[slot], nil
}

// Live reports whether bucketKey is active for the named cohort. A
// cohort with active_every in {0, 1} is always live; otherwise bucket
// is live iff bucketKey % active_every == 0.
func (r *Registry) Live(name string, bucketKey int64) (bool, error) {
	sched, ok := r.schedules[name]
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnknownCohort, name)
	}

	every := sched.activeEvery
	if every <= 1 {
		return true, nil
	}

	return bucketKey%every == 0, nil
}

// compileSchedule validates one Cohort and wraps it with a fresh cache.
func compileSchedule(pb *dgproto.Cohort, cacheSize int) (*schedule, error) {
	name := pb.GetName()
	if name == "" {
		return nil, fmt.Errorf("%w: empty name", ErrInvalidCohort)
	}

	cohortSize := pb.GetCohortSize()
	if cohortSize <= 0 {
		return nil, fmt.Errorf("%w: %q cohort_size %d",
			ErrInvalidCohort, name, cohortSize)
	}

	lo, hi := pb.GetEntityMin(), pb.GetEntityMax()
	if lo > hi {
		return nil, fmt.Errorf("%w: %q [%d, %d]",
			ErrInvalidRange, name, lo, hi)
	}

	span := hi - lo + 1
	if cohortSize > span {
		return nil, fmt.Errorf("%w: %q cohort_size %d > span %d",
			ErrCohortTooLarge, name, cohortSize, span)
	}

	ratio := pb.GetPersistenceRatio()
	if ratio < 0 || ratio > 1 {
		return nil, fmt.Errorf("%w: %q persistence_ratio %v",
			ErrInvalidCohort, name, ratio)
	}

	if pb.GetActiveEvery() < 0 {
		return nil, fmt.Errorf("%w: %q active_every %d",
			ErrInvalidCohort, name, pb.GetActiveEvery())
	}

	if pb.GetPersistenceMod() < 0 {
		return nil, fmt.Errorf("%w: %q persistence_mod %d",
			ErrInvalidCohort, name, pb.GetPersistenceMod())
	}

	return &schedule{
		name:             name,
		cohortSize:       cohortSize,
		entityMin:        lo,
		entityMax:        hi,
		span:             span,
		activeEvery:      pb.GetActiveEvery(),
		persistenceMod:   pb.GetPersistenceMod(),
		persistenceRatio: ratio,
		seedSalt:         pb.GetSeedSalt(),
		cache:            newSlotCache(cacheSize),
	}, nil
}

// slotsFor returns the materialized slot list for (schedule, bucketKey),
// hitting the LRU or computing a fresh Fisher-Yates permutation on a
// miss.
func (r *Registry) slotsFor(sched *schedule, bucketKey int64) []int64 {
	if slots, ok := sched.cache.get(bucketKey); ok {
		return slots
	}

	slots := r.buildSlots(sched, bucketKey)
	sched.cache.put(bucketKey, slots)

	return slots
}

// buildSlots computes the ordered list of cohort_size entity IDs for a
// bucket. When persistence is enabled (persistence_mod > 0 AND
// persistence_ratio > 0) the first `persistentCount` slots are seeded
// by (bucket_key mod persistence_mod); the remaining slots are seeded
// by bucket_key and drawn from entities not already chosen for the
// persistent prefix.
//
// Algorithm: two staged Fisher-Yates partial shuffles over the
// [entity_min, entity_max] pool.
//  1. persist_seed = Derive(root, "cohort", name, "mod", k_mod, salt)
//     drives a partial FY that yields the first persistentCount slots.
//  2. absolute_seed = Derive(root, "cohort", name, "abs", k_abs, salt)
//     drives a partial FY over the remaining pool (entities not taken
//     by the persistent prefix) yielding the tail slots.
func (r *Registry) buildSlots(sched *schedule, bucketKey int64) []int64 {
	size := int(sched.cohortSize)
	pool := make([]int64, sched.span)

	for i := range pool {
		pool[i] = sched.entityMin + int64(i)
	}

	persistentCount := persistentCount(sched)
	slots := make([]int64, 0, size)
	// effective is the "unchosen head" length of pool. partialShuffle
	// swaps drawn entries to positions >= effective so the next pass
	// picks from the remaining head.
	effective := len(pool)

	if persistentCount > 0 {
		persistSeed := r.deriveSeed(sched, "mod", bucketKey%sched.persistenceMod)
		prng := seed.PRNG(persistSeed)
		slots, effective = partialShuffle(prng, pool, effective, persistentCount, slots)
	}

	remaining := size - persistentCount
	if remaining > 0 {
		absSeed := r.deriveSeed(sched, "abs", bucketKey)
		prng := seed.PRNG(absSeed)
		slots, _ = partialShuffle(prng, pool, effective, remaining, slots)
	}

	return slots
}

// deriveSeed composes the sub-seed for a given (schedule, kind, key)
// triple. The "cohort" prefix keeps schedule derivations in their own
// namespace; seed_salt on the Cohort buys independence across
// schedules that share the same entity range.
func (r *Registry) deriveSeed(sched *schedule, kind string, key int64) uint64 {
	return seed.Derive(
		r.rootSeed,
		"cohort",
		sched.name,
		kind,
		strconv.FormatInt(key, 10),
		strconv.FormatUint(sched.seedSalt, 10),
	)
}

// persistentCount returns floor(cohort_size * persistence_ratio) when
// persistence is enabled, else 0.
func persistentCount(sched *schedule) int {
	if sched.persistenceMod <= 0 || sched.persistenceRatio <= 0 {
		return 0
	}

	count := int(float32(sched.cohortSize) * sched.persistenceRatio)
	if count < 0 {
		count = 0
	}

	if count > int(sched.cohortSize) {
		count = int(sched.cohortSize)
	}

	return count
}

// partialShuffle appends `count` entries drawn from pool[:effective]
// via Fisher-Yates without replacement to `into`. Drawn elements are
// swapped to the tail so the remaining head carries the unchosen
// entries.
func partialShuffle(
	prng *rand.Rand, pool []int64, effective, count int, into []int64,
) (out []int64, newEffective int) {
	for i := 0; i < count && effective > 0; i++ {
		// Pick a random element from [0, effective) and swap it to the
		// tail so the head of length effective-1 still holds the
		// unchosen elements.
		j := prng.IntN(effective)
		into = append(into, pool[j])

		effective--
		pool[j], pool[effective] = pool[effective], pool[j]
	}

	return into, effective
}

// slotCache is a bounded LRU mapping bucketKey → materialized slot list.
type slotCache struct {
	cap   int
	order *list.List
	index map[int64]*list.Element
}

// cacheEntry binds a bucket key to its slot list.
type cacheEntry struct {
	key   int64
	slots []int64
}

// newSlotCache returns a cache with at least 1 slot of capacity.
func newSlotCache(capacity int) *slotCache {
	if capacity < 1 {
		capacity = 1
	}

	return &slotCache{
		cap:   capacity,
		order: list.New(),
		index: make(map[int64]*list.Element, capacity),
	}
}

// get promotes and returns the cached slot list for key, or reports miss.
func (c *slotCache) get(key int64) ([]int64, bool) {
	elem, ok := c.index[key]
	if !ok {
		return nil, false
	}

	c.order.MoveToFront(elem)

	entry, _ := elem.Value.(*cacheEntry)

	return entry.slots, true
}

// put inserts (key, slots) as the MRU entry, evicting the LRU entry
// when the cap is already reached.
func (c *slotCache) put(key int64, slots []int64) {
	if _, ok := c.index[key]; ok {
		return
	}

	if c.order.Len() >= c.cap {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)

			entry, _ := oldest.Value.(*cacheEntry)
			delete(c.index, entry.key)
		}
	}

	elem := c.order.PushFront(&cacheEntry{key: key, slots: slots})
	c.index[key] = elem
}

// Len returns the number of cached buckets across all schedules.
// Test-only; callers should not rely on eviction ordering.
func (r *Registry) Len(name string) int {
	sched, ok := r.schedules[name]
	if !ok {
		return 0
	}

	return sched.cache.order.Len()
}
