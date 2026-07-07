package bench

import "github.com/stroppy-io/stroppy/next/rng"

// DeriveStream is the bench-layer surface of [rng.Derive]: it builds a
// deterministic, seekable rng stream from the run root seed and a (stepID,
// streamID) pair. It is the canonical way for a test to derive its own
// deterministic streams — run-global agreement streams, TPC-C NURand run
// constants, anything that must share the single root seed exposed by --seed —
// so a private seed literal can never again fall outside the flag's reach.
//
// Identical (seed, stepID, streamID) always yields the identical stream
// (rng.Derive's compatibility contract). Use distinct stepIDs to keep a test's
// constant streams separate from every step's data streams; the FNV-32a of a
// step name (the value a VU's rng derives from) is non-zero for any real name,
// so a constant step id of 0 or a fixed non-name sentinel is safely distinct.
func DeriveStream(seed uint64, stepID, streamID uint32) rng.Stream {
	return rng.Derive(seed, stepID, streamID)
}
