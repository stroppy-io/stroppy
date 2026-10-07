package dsqgen

// rng is a small deterministic PRNG used to pick valid parameter values. It is
// the Lehmer MINSTD generator (the same family dsqgen uses) but the generator
// does NOT reproduce dsqgen's exact stream — it only needs reproducible,
// well-distributed draws for a given seed. Distinct query streams use distinct
// seeds.
type rng struct{ seed int64 }

const (
	rngMult   = 16807
	rngMaxInt = 2147483647 // 2^31 - 1
	rngQ      = rngMaxInt / rngMult
	rngR      = rngMaxInt % rngMult
)

func newRNG(seed int64) *rng {
	s := seed % rngMaxInt
	if s <= 0 {
		s += rngMaxInt - 1
	}
	return &rng{seed: s}
}

// next advances the stream (Schrage's method) and returns a value in
// [1, rngMaxInt).
func (r *rng) next() int64 {
	div := r.seed / rngQ
	mod := r.seed - rngQ*div
	s := rngMult*mod - div*rngR
	if s < 0 {
		s += rngMaxInt
	}
	r.seed = s
	return s
}

// intn returns a uniform integer in [min, max] inclusive (min <= max).
func (r *rng) intn(min, max int64) int64 {
	if max <= min {
		return min
	}
	return min + r.next()%(max-min+1)
}
