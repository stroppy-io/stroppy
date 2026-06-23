package dsdgen

// businessKeyChars are the 16 symbols used to encode surrogate business keys
// (one hex nibble each). From BusinessKeyGenerator.java / build_support.c.
const businessKeyChars = "ABCDEFGHIJKLMNOP"

// MakeBusinessKey encodes primary as the 16-char TPC-DS business key:
// the high 32 bits then the low 32 bits, each as 8 chars, least-significant
// nibble first. E.g. MakeBusinessKey(1) == "AAAAAAAABAAAAAAA".
func MakeBusinessKey(primary int64) string {
	var b [16]byte
	encode8(b[0:8], primary>>32)
	encode8(b[8:16], primary)

	return string(b[:])
}

func encode8(dst []byte, value int64) {
	for i := 0; i < 8; i++ {
		dst[i] = businessKeyChars[value&0xF]
		value >>= 4
	}
}
