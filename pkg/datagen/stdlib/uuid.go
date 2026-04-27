package stdlib

import (
	"encoding/binary"
	"fmt"

	"github.com/google/uuid"

	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// uuidByteLen is the fixed octet length of a v4 UUID.
const uuidByteLen = 16

// uuidVersionByte is the octet index (6) holding the 4-bit version nibble.
const uuidVersionByte = 6

// uuidVariantByte is the octet index (8) holding the 2-bit variant bits.
const uuidVariantByte = 8

// uuidVersionMaskClear clears the top nibble; uuidVersion4Bits sets v4.
const (
	uuidVersionMaskClear = 0x0F
	uuidVersion4Bits     = 0x40
	uuidVariantMaskClear = 0x3F
	uuidVariantRFC4122   = 0x80
)

func init() {
	registry["std.uuidSeeded"] = uuidSeeded
}

// uuidSeeded implements `std.uuidSeeded(seed int64) → string`. The UUID
// is derived by filling 16 bytes from seed.PRNG(uint64(seed)) and then
// forcing the v4 version and RFC 4122 variant nibbles. The result is
// deterministic for a given seed and stable across platforms because
// seed.PRNG is backed by a PCG source with a fixed stream formula.
func uuidSeeded(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: std.uuidSeeded needs 1, got %d", ErrArity, len(args))
	}

	key, ok := toInt64(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: std.uuidSeeded arg 0: expected int64, got %T", ErrArgType, args[0])
	}

	prng := seed.PRNG(uint64(key)) //nolint:gosec // bit reinterpret is intentional

	// Fill two 64-bit words and encode them little-endian into the 16-byte
	// buffer. The explicit encoder keeps the byte order stable across
	// platforms without introducing unchecked uint32→byte conversions.
	var raw [uuidByteLen]byte
	binary.LittleEndian.PutUint64(raw[:8], prng.Uint64())
	binary.LittleEndian.PutUint64(raw[8:], prng.Uint64())

	raw[uuidVersionByte] = (raw[uuidVersionByte] & uuidVersionMaskClear) | uuidVersion4Bits
	raw[uuidVariantByte] = (raw[uuidVariantByte] & uuidVariantMaskClear) | uuidVariantRFC4122

	return uuid.UUID(raw).String(), nil
}
