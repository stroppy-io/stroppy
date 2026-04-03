package randstr

import "testing"

func BenchmarkStringGenerator_Next(b *testing.B) {
	sg := NewStringGenerator(42, &MockDistribution[uint64]{Values: []uint64{10}}, nil, 10)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		sg.Next()
	}
}

func BenchmarkCharTape_Next(b *testing.B) {
	ct := NewCharTape(42, DefaultEnglishAlphabet)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ct.Next()
	}
}

func BenchmarkWordCutter_Cut(b *testing.B) {
	wc := NewWordCutter(&MockDistribution[uint64]{Values: []uint64{10}}, 10, NewCharTape(42, DefaultEnglishAlphabet))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		wc.Cut()
	}
}
