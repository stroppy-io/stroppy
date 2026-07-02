// Package rng provides the counter-based PRNG and seed derivation that back
// the engine's determinism contract: a stream is derived from (root seed,
// step id, stream id), and any cycle of it is reachable in O(1).
package rng
