package generate

import (
	"crypto/rand"
	"encoding/binary"
)

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

	return binary.BigEndian.Uint64(b[:])
}
