package generate

import (
	"testing"

	pb "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// Benchmarks measure allocs/op — the key metric for GC pressure.
// Run before and after each optimization pass and compare with benchstat.

func BenchmarkGenerator_Int32(b *testing.B) {
	gen, _ := NewValueGeneratorByRule(42, &pb.Generation_Rule{
		Kind: &pb.Generation_Rule_Int32Range{
			Int32Range: &pb.Generation_Range_Int32{Max: 1_000_000},
		},
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = gen.Next()
	}
}

func BenchmarkGenerator_Float32(b *testing.B) {
	gen, _ := NewValueGeneratorByRule(42, &pb.Generation_Rule{
		Kind: &pb.Generation_Rule_FloatRange{
			FloatRange: &pb.Generation_Range_Float{Max: 1_000_000},
		},
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = gen.Next()
	}
}

func BenchmarkGenerator_Int64(b *testing.B) {
	gen, _ := NewValueGeneratorByRule(42, &pb.Generation_Rule{
		Kind: &pb.Generation_Rule_Int64Range{
			Int64Range: &pb.Generation_Range_Int64{Max: 1_000_000},
		},
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = gen.Next()
	}
}

func BenchmarkGenerator_String(b *testing.B) {
	gen, _ := NewValueGeneratorByRule(42, &pb.Generation_Rule{
		Kind: &pb.Generation_Rule_StringRange{
			StringRange: &pb.Generation_Range_String{MaxLen: 20},
		},
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = gen.Next()
	}
}

func BenchmarkGenerator_DateTime(b *testing.B) {
	gen, _ := NewValueGeneratorByRule(42, &pb.Generation_Rule{
		Kind: &pb.Generation_Rule_DatetimeRange{
			DatetimeRange: &pb.Generation_Range_DateTime{
				Type: &pb.Generation_Range_DateTime_Timestamp{
					Timestamp: &pb.Generation_Range_DateTime_TimestampUnix{
						Min: 0,
						Max: 1_000_000_000,
					},
				},
			},
		},
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = gen.Next()
	}
}

func BenchmarkGenerator_Decimal(b *testing.B) {
	gen, _ := NewValueGeneratorByRule(42, &pb.Generation_Rule{
		Kind: &pb.Generation_Rule_DecimalRange{
			DecimalRange: &pb.Generation_Range_DecimalRange{
				Type: &pb.Generation_Range_DecimalRange_Float{
					Float: &pb.Generation_Range_Float{Max: 1_000_000},
				},
			},
		},
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = gen.Next()
	}
}
