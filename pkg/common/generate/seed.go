package generate

import "crypto/rand"

// ResolveSeed resolves a seed value with the semantic: 0 = random, >0 = fixed.
// Callers should always pass seeds through ResolveSeed before using them.
func ResolveSeed(s uint64) uint64 {
	if s != 0 {
		return s
	}

	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("generate.ResolveSeed: crypto/rand unavailable: " + err.Error())
	}

	return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
}
