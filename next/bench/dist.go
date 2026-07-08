package bench

import (
	"math"

	"github.com/stroppy-io/stroppy/next/rng"
)

// Distribution primitives over (stream, cycle): thin pure-kernel wrappers,
// allocation-free, with our own readable names (not v5's relational-algebra
// vocabulary). Each is a function, not a value type — no setup, no allocator,
// no opaque state to persist. Generators that need something these do not
// express write plain Go over [rng] kernels; that is first-class, not an escape
// hatch (D8: the SDK is not the ceiling).
//
// Normal and Decimal ship now (the S1/S4 needs); Zipf, Phrase, Dict, Grammar,
// Date and Bernoulli defer until tpch/tpcds lift them, designed for the
// workload that needs them rather than before it.

// Decimal returns a float64 uniformly in [lo, hi] with scale decimal places of
// precision. The draw is over the integer range [round(lo*10^scale),
// round(hi*10^scale)], so every value lands exactly on a representable decimal
// at the given scale — no float rounding noise in the generated data, and no
// reliance on the DB column to truncate. Drawn from (s, cycle) at sub 0;
// allocation-free.
//
// lo and hi are expressed in the same scale as the result, e.g.
// Decimal(s, cy, 0.01, 9999.99, 2) for a monetary amount with cents. scale<=0
// yields whole-number draws over [round(lo), round(hi)].
func Decimal(s rng.Stream, cycle uint64, lo, hi float64, scale int) float64 {
	if scale < 0 {
		scale = 0
	}
	p := math.Pow(10, float64(scale))
	loInt := int64(math.Round(lo * p))
	hiInt := int64(math.Round(hi * p))
	return float64(rng.UniformInt(s, cycle, loInt, hiInt)) / p
}

// Normal returns a normally-distributed float64 with the given mean and standard
// deviation, via the Box-Muller transform over two uniform draws (subs 0 and 1)
// of (s, cycle). Allocation-free; pure in (s, cycle).
//
// Box-Muller: z = r*cos(2*pi*u2), r = sqrt(-2*ln(u1)). u1 is clamped off zero to
// keep the log finite; the clamp does not bias any practical tail.
func Normal(s rng.Stream, cycle uint64, mean, stddev float64) float64 {
	u1 := rng.UniformFloatSub(s, cycle, 0)
	if u1 < 1e-300 {
		u1 = 1e-300
	}
	u2 := rng.UniformFloatSub(s, cycle, 1)
	r := math.Sqrt(-2 * math.Log(u1))
	theta := 2 * math.Pi * u2
	return mean + stddev*r*math.Cos(theta)
}
