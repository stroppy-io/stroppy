package mem

import "unsafe"

// Arena is a chunked bump allocator for variable-size hot-path data (RFC 0001
// §6). Alloc(n) hands out a slice by advancing an offset within the current
// fixed-size chunk; when a chunk is exhausted it moves to the next, appending a
// new chunk only when none is spare. Reset rewinds to the first chunk without
// freeing anything, so after a warm-up pass the steady state allocates nothing:
// the same fill pattern reuses the same chunks batch after batch.
//
// It is not safe for concurrent use; give each VU its own Arena.
type Arena struct {
	chunks  [][]byte
	cur     int // index of the current chunk
	off     int // bump offset within chunks[cur]
	chunkSz int // size of a regular chunk
}

// NewArena returns an arena whose regular chunks are chunkSz bytes. chunkSz
// should comfortably exceed the largest single Alloc a batch makes; requests
// larger than chunkSz get their own oversized chunk (see Alloc). The first
// chunk is preallocated so a single-chunk batch never grows.
func NewArena(chunkSz int) *Arena {
	if chunkSz <= 0 {
		panic("mem: NewArena chunkSz must be positive")
	}

	return &Arena{
		chunks:  [][]byte{make([]byte, chunkSz)},
		chunkSz: chunkSz,
	}
}

// Alloc returns a zeroed-length-n slice backed by arena memory, valid until the
// next Reset. It advances the bump offset; it allocates only when it must grow
// (no spare chunk, or an oversized request). Steady-state reuse — the common
// case — is allocation-free.
//
// The returned slice's cap is exactly n (a three-index slice), so appending to
// it cannot silently spill into the next allocation's bytes.
func (a *Arena) Alloc(n int) []byte {
	if n < 0 {
		panic("mem: Arena.Alloc negative size")
	}

	if n > a.chunkSz {
		return a.allocOversized(n)
	}

	if a.off+n > len(a.chunks[a.cur]) {
		a.nextChunk()
	}

	start := a.off
	a.off += n

	return a.chunks[a.cur][start : start+n : start+n]
}

// nextChunk advances to the next chunk, appending a fresh regular chunk when no
// spare exists. The append is the only steady-state-avoidable allocation, and
// only happens while the arena is still growing to its high-water mark.
func (a *Arena) nextChunk() {
	a.cur++
	a.off = 0

	if a.cur == len(a.chunks) {
		a.chunks = append(a.chunks, make([]byte, a.chunkSz))
	}

	// A spare chunk from a previous, larger batch may be oversized; that is
	// fine — Alloc only ever bumps within len(chunk).
}

// allocOversized serves a request larger than chunkSz from its own dedicated
// chunk placed at the current position. Such chunks are not pooled by size, so
// an oversized Alloc allocates unless an equally-large spare already sits here;
// size chunkSz to keep these off the hot path.
func (a *Arena) allocOversized(n int) []byte {
	a.nextChunk()

	if len(a.chunks[a.cur]) < n {
		a.chunks[a.cur] = make([]byte, n)
	}

	a.off = n

	return a.chunks[a.cur][0:n:n]
}

// Reset rewinds the arena to its start, keeping all chunks for reuse. Every
// slice previously returned by Alloc (and every String view over them) is
// invalid after Reset.
func (a *Arena) Reset() {
	a.cur = 0
	a.off = 0
}

// Cap reports the total bytes currently backed by the arena's chunks.
func (a *Arena) Cap() int {
	total := 0
	for _, c := range a.chunks {
		total += len(c)
	}

	return total
}

// String returns a string sharing b's backing bytes with no copy. It is only
// valid while b is valid — i.e. until the arena's next Reset. Use it for
// transient views (e.g. a key passed straight to a driver bind buffer); never
// retain the result past the batch. Mutating the arena bytes after taking the
// view breaks Go's string-immutability assumption, so treat b as frozen once
// viewed.
func (a *Arena) String(b []byte) string {
	if len(b) == 0 {
		return ""
	}

	return unsafe.String(&b[0], len(b))
}
