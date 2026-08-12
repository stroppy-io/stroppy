package gen

import "math/bits"

// Counter-based deterministic derivation.
//
// A Field's value at an entity is a pure function of (field state, entity,
// sub):
//
//	raw(entity, sub) = SplitMix64(seed0 + entity*gamma + sub*subGamma)
//
// SplitMix64 is the Steele/Lea/Flood 2014 bit-mixer (a bijection with strong
// avalanche). Each field owns its own seed0 and an odd gamma so fields
// decorrelate rather than being shifted views of one sequence; subGamma is a
// second odd increment so multi-word values (filled strings, NURand) walk a
// different stride than successive entities. Any entity is reachable in O(1)
// with no sequential state.

// smix* are the SplitMix64 round constants (Steele, Lea, Flood 2014).
const (
	smixGamma  = 0x9E3779B97F4A7C15 // golden-ratio odd increment
	smixMul1   = 0xBF58476D1CE4E5B9
	smixMul2   = 0x94D049BB133111EB
	smixShift1 = 30
	smixShift2 = 27
	smixShift3 = 31
)

// subGamma is the odd Weyl increment for sub-draws within one entity. It is
// an unrelated odd constant (the Fibonacci-hashing multiplier used by
// wyhash) so a multi-word value's sub-draws walk a different stride than the
// entity counter.
const subGamma = 0xD1B54A32D192ED03

// SplitMix64 is the full splitmix64 bit-mixer (5 XORs + 2 multiplies), the
// single mixing primitive for both derivation and the counter core. It is a
// bijection with BigCrush-passing avalanche. Exported so imperative generators
// can mix a derived key directly (for example TPC-C NURand).
func SplitMix64(x uint64) uint64 {
	x += smixGamma
	x = (x ^ (x >> smixShift1)) * smixMul1
	x = (x ^ (x >> smixShift2)) * smixMul2

	return x ^ (x >> smixShift3)
}

// Root is the deterministic base for a run. It is a small immutable value: it
// is safe to copy and to share across goroutines.
//
// Construct one with New at the start of a workload and derive domains and
// fields from it. The zero Root is a valid deterministic root for seed 0.
type Root struct{ seed uint64 }

// New returns the Root for seed. seed 0 is valid and deterministic.
func New(seed uint64) Root { return Root{seed: seed} }

// Seed returns the root seed this Root was constructed from.
func (r Root) Seed() uint64 { return r.seed }

// Domain derives a named namespace from the root. The same (root, name) pair
// always yields the same domain, independent of declaration order or of how
// many other domains exist. name should be workload-owned and carry a version
// suffix (for example "tpcc/item@1") so that a workload's data is stable
// across execution-graph edits and so an intentional formula change can
// increment the version.
//
// Domain is immutable and safe to copy and share.
func (r Root) Domain(name string) Domain {
	return Domain{seed: SplitMix64(r.seed ^ fnv1a64(name))}
}

// Domain is a namespace within a Root. Fields derived from a domain are
// independent of fields derived from any other domain or from the root.
//
// The zero Domain is a valid domain for the empty name under seed 0.
type Domain struct{ seed uint64 }

// Field derives a per-field stream from the domain. The same (domain, name)
// pair always yields the same field; distinct names yield independent
// streams. Editing one field's name never shifts another field's sequence
// (the way a hand-numbered stream-id list would).
//
// Field is immutable and safe to copy and share across goroutines.
func (d Domain) Field(name string) Field {
	key := SplitMix64(d.seed ^ fnv1a64(name))

	return Field{
		seed0: SplitMix64(key),
		gamma: SplitMix64(key^smixGamma) | 1, // force odd → full-period Weyl walk
	}
}

// Field is a derived, read-only random sequence scoped to one domain and one
// logical field. It is a small immutable value: safe to copy and share.
//
// One-word kernels (Uint64, Int64, Int, Float64, Decimal, Chance) are pure
// methods on Field taking an explicit entity index; no cursor, no mutable
// state. Multi-word kernels (Fill, Bytes) advance a *Draw cursor across the
// entity's sub-draws; use them for a single field that must yield more than
// one word (a filled string). Do not mix the two on one field: a one-word
// kernel draws on a local copy and does not advance the caller's cursor, so
// draw a random length from a separate field, or use Bytes which draws the
// length and fills in one sequenced pass.
type Field struct {
	seed0 uint64 // Weyl start
	gamma uint64 // per-field odd Weyl increment
}

// At returns a Draw positioned at entity. A Draw is the stack-only cursor a
// multi-word kernel uses to take several values for a single entity of this
// field. Distinct entities of the same field are independent; seeking is
// selecting a different entity, not replaying prior draws.
func (f Field) At(entity uint64) Draw { return Draw{f: f, entity: entity} }

// Draw is a stack-only cursor for the values of one field at one entity.
//
// Multi-word kernels (Fill, Bytes) advance the cursor's sub-draw across
// positions; a Draw is therefore not safe for concurrent use by multiple
// goroutines. Copy it per goroutine (copies are cheap and independent).
type Draw struct {
	f      Field
	entity uint64
	sub    uint64
}

// raw returns the 64-bit output for the current entity at the current
// sub-draw, then advances the sub-draw cursor.
func (d *Draw) raw() uint64 {
	v := SplitMix64(d.f.seed0 + d.entity*d.f.gamma + d.sub*subGamma)
	d.sub++

	return v
}

// uniform maps the cursor's next sub-draw into [0, n) exactly, using Daniel
// Lemire's multiply-high method with rejection on the biased tail. Each
// rejected candidate consumes one additional sub-draw.
func (d *Draw) uniform(n uint64) uint64 {
	if n <= 1 {
		_ = d.raw() // still consume a sub-draw so positions stay sequenced

		return 0
	}

	_, r := bits.Div64(1, 0, n)
	for {
		hi, lo := bits.Mul64(d.raw(), n)
		if lo >= r {
			return hi
		}
	}
}
