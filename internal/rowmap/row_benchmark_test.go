package rowmap

import "testing"

func BenchmarkDecodeInt64(b *testing.B) {
	values := []any{int64(123456789)}

	b.ReportAllocs()

	for b.Loop() {
		value, err := Decode[int64](nil, values)
		if err != nil || value != 123456789 {
			b.Fatalf("Decode() = %v, %v", value, err)
		}
	}
}

func BenchmarkDecodeTextInt64(b *testing.B) {
	values := []any{"123456789"}

	b.ReportAllocs()

	for b.Loop() {
		value, err := Decode[int64](nil, values)
		if err != nil || value != 123456789 {
			b.Fatalf("Decode() = %v, %v", value, err)
		}
	}
}

func BenchmarkDecodeBytes(b *testing.B) {
	values := []any{[]byte("hello world")}

	b.ReportAllocs()

	for b.Loop() {
		value, err := Decode[[]byte](nil, values)
		if err != nil || len(value) != 11 {
			b.Fatalf("Decode() = %v, %v", value, err)
		}
	}
}
