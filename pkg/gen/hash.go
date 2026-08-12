package gen

// 64-bit FNV-1a, used to fold a name into a uint64 key. Allocation-free and
// inline; it is the single source of truth for name hashing in this package
// so a domain or field identity is a pure function of its name.
const (
	fnvOffset64 uint64 = 0xCBF29CE484222325
	fnvPrime64  uint64 = 0x100000001B3
)

func fnv1a64(s string) uint64 {
	h := fnvOffset64
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}

	return h
}
