package bench

import (
	"hash/fnv"

	"github.com/stroppy-io/stroppy/next/rng"
)

// Named streams replace the hand-numbered stream-id footgun: instead of
// vu.Rand(0), vu.Rand(1), ... with nStreams coupled by convention, a generator
// asks for a stream by name and the SDK maps the name to a stable id. The
// mapping is the 32-bit FNV-1a of the name — the same hash [stepID] uses for
// step names — so a stream's identity is a pure function of its name, identical
// across runs and independent of declaration order. Renaming a stream changes
// its draws (as it should); inserting or reordering streams does not.
//
// One namespace per step: stream ids derive under the step's id
// ([rng.Derive](rootSeed, stepID, streamID)), so the same name under two
// different steps reaches two independent streams. A 32-bit hash space is wide
// enough that collisions between distinct names in one step are astronomically
// unlikely (the same property [stepID] already relies on for step names).

// StreamID returns the deterministic rng stream id for name: the 32-bit FNV-1a
// hash of the name. Authors who write custom handlers (not using [Loader]) pass
// the result to [VU.Rand]. The [Loader] path hands a generator a [*Streams] that
// calls this under the hood.
//
// The id feeds [VU.Rand] / [rng.Derive] unchanged — identical names reach
// identical streams, so the rng compatibility contract (D11) holds.
func StreamID(name string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return h.Sum32()
}

// Streams is the per-step namespace of named rng streams handed to a generator
// ([GenFn]). Each name resolves to the same stream [VU.Rand] would produce for
// [StreamID](name) under this step's id and the run seed; a generator that asks
// for "i_im_id" draws the same sequence inside a [Loader] handler or a
// hand-rolled one that calls [VU.Rand]([StreamID]("i_im_id")).
//
// Streams are cached on first lookup, so steady-state reads (every name seen)
// are a map read — allocation-free. Build one per VU with [StreamsFrom] in Init,
// or driverless with [NewStreams] in tests.
type Streams struct {
	stepID   uint32
	rootSeed uint64
	cache    map[string]rng.Stream
}

// StreamsFrom builds a namespace over the VU's rng derivation. Call it once per
// VU in Init; the returned *Streams is reused across every cycle the generator
// draws. It is the constructor a [Loader] handler uses.
func StreamsFrom(vu *VU) *Streams {
	return &Streams{
		stepID:   vu.stepID,
		rootSeed: vu.rootSeed,
		cache:    make(map[string]rng.Stream),
	}
}

// NewStreams builds a namespace directly from a root seed and a step name,
// deriving the step id via FNV-1a the way the engine does at run time. It is
// the driverless counterpart to [StreamsFrom] for tests and other callers
// without a [VU]: a generator run under NewStreams(seed, "load_item") produces
// byte-identical draws to the same generator driven by a "load_item" step.
func NewStreams(seed uint64, stepName string) *Streams {
	return &Streams{
		stepID:   stepID(stepName),
		rootSeed: seed,
		cache:    make(map[string]rng.Stream),
	}
}

// Stream returns the rng stream for name, deriving and memoizing it on first
// call. Repeated calls for the same name are a map read; the hot path is
// allocation-free once every name has been seen.
func (s *Streams) Stream(name string) rng.Stream {
	if r, ok := s.cache[name]; ok {
		return r
	}
	r := rng.Derive(s.rootSeed, s.stepID, StreamID(name))
	s.cache[name] = r
	return r
}
