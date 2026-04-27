// Package cohort compiles Cohort schedules into a Registry that answers
// deterministic entity-slot draws per bucket key. Schedules are stateless
// pure functions of (root_seed, name, bucket_key, slot); the Registry
// caches recently-seen buckets in a bounded LRU but never relies on
// accumulated state. Selection is Fisher-Yates partial shuffle over the
// inclusive [entity_min, entity_max] range; persistence splits the
// cohort into a (bucket_key mod persistence_mod)-seeded prefix and a
// bucket_key-seeded remainder.
package cohort

import "errors"

// ErrUnknownCohort is returned by Draw/Live when the requested schedule
// name is not present in the Registry.
var ErrUnknownCohort = errors.New("cohort: unknown schedule")

// ErrSlotRange is returned by Draw when the requested slot is negative
// or >= cohort_size.
var ErrSlotRange = errors.New("cohort: slot out of [0, cohort_size)")

// ErrInvalidRange is returned by New when a Cohort declares
// entity_min > entity_max.
var ErrInvalidRange = errors.New("cohort: entity_min > entity_max")

// ErrCohortTooLarge is returned by New when a Cohort declares
// cohort_size larger than the span (entity_max - entity_min + 1).
var ErrCohortTooLarge = errors.New("cohort: cohort_size exceeds span")

// ErrDuplicateCohort is returned by New when two Cohort entries share
// the same name.
var ErrDuplicateCohort = errors.New("cohort: duplicate schedule name")

// ErrInvalidCohort is returned by New when a Cohort carries a blank
// name, non-positive cohort_size, or a persistence_ratio outside [0, 1].
var ErrInvalidCohort = errors.New("cohort: invalid schedule")
