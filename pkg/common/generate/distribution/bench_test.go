package distribution

import "testing"

func BenchmarkUniformDistribution_Next_Float(b *testing.B) {
	ud := NewUniformDistribution(42, [2]float64{0, 1_000_000}, false, 0)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ud.Next()
	}
}

func BenchmarkUniformDistribution_Next_Round(b *testing.B) {
	ud := NewUniformDistribution(42, [2]int64{0, 1_000_000}, true, 0)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ud.Next()
	}
}

func BenchmarkUniqueNumberGenerator_Next(b *testing.B) {
	gen := NewUniqueDistribution[int64]([2]int64{0, 1 << 50})
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		gen.Next()
	}
}

func BenchmarkUniqueNumberGenerator_Next_Parallel(b *testing.B) {
	gen := NewUniqueDistribution[int64]([2]int64{0, 1 << 60})
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			gen.Next()
		}
	})
}
