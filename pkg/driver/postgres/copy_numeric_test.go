package postgres

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestCopyNumericEncoding(t *testing.T) {
	t.Parallel()

	original := pgtype.NewMap()
	pgxdecimal.Register(original)
	fast := original.Copy()
	typ, ok := fast.TypeForOID(pgtype.NumericOID)
	require.True(t, ok)
	fast.RegisterType(&pgtype.Type{Name: typ.Name, OID: typ.OID, Codec: copyNumericCodec{Codec: typ.Codec}})

	texts := []string{
		"0", "0.00", "-0", "-0.00", "+000", "123", "10000", "100000000",
		"0.0001", ".1", "1.", "-123.45", "000123.4500", "0.00000001",
		"9223372036854775807.99", "-9223372036854775808.00",
		strings.Repeat("9", 64), "0." + strings.Repeat("0", 60) + "1",
		"1e5", "-1.25e-8", strings.Repeat("9", 80),
	}

	cases := make([]string, len(texts), len(texts)+20000)
	copy(cases, texts)
	texts = cases

	rng := rand.New(rand.NewPCG(1, 2))
	for range 10000 {
		texts = append(texts, fmt.Sprintf("%d.%04d", rng.Int64(), rng.IntN(10000)))
		length := rng.IntN(63) + 1

		digits := make([]byte, length)
		for i := range digits {
			digits[i] = byte('0' + rng.IntN(10))
		}

		dot := rng.IntN(length + 1)
		texts = append(texts, string(digits[:dot])+"."+string(digits[dot:]))
	}

	for _, text := range texts {
		got, err := fast.Encode(pgtype.NumericOID, pgtype.BinaryFormatCode, copyText(text), nil)
		require.NoError(t, err, text)
		want, err := original.Encode(pgtype.NumericOID, pgtype.BinaryFormatCode, copyText(text), nil)
		require.NoError(t, err, text)

		var gotValue, wantValue string
		require.NoError(t, fast.Scan(pgtype.NumericOID, pgtype.BinaryFormatCode, got, &gotValue), text)
		require.NoError(t, original.Scan(pgtype.NumericOID, pgtype.BinaryFormatCode, want, &wantValue), text)
		require.Equal(t, wantValue, gotValue, text)
		require.Equal(t, binary.BigEndian.Uint16(want[6:8]), binary.BigEndian.Uint16(got[6:8]), text)
	}
}

func TestCopyNumericFallback(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "+", "-", ".", "1.2.3", "1e4", "NaN", "Infinity", " 1", "1 ", "x"} {
		prefix := []byte{1, 2, 3}
		got, ok := appendCopyNumeric(prefix, text)
		require.False(t, ok, text)
		require.Equal(t, prefix, got, text)
	}
}

func BenchmarkCopyNumeric(b *testing.B) {
	for _, fast := range []bool{false, true} {
		b.Run(fmt.Sprintf("fast=%t", fast), func(b *testing.B) {
			m := pgtype.NewMap()
			pgxdecimal.Register(m)

			if fast {
				typ, _ := m.TypeForOID(pgtype.NumericOID)
				m.RegisterType(&pgtype.Type{Name: typ.Name, OID: typ.OID, Codec: copyNumericCodec{Codec: typ.Codec}})
			}

			value := copyText("12345.67")
			buf := make([]byte, 0, 32)

			b.ReportAllocs()

			for b.Loop() {
				var err error

				buf, err = m.Encode(pgtype.NumericOID, pgtype.BinaryFormatCode, value, buf[:0])
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
