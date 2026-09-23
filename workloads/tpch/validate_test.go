package tpch

import "testing"

func TestCompareQueryBoundsMismatchDetails(t *testing.T) {
	got := make([][]any, maxDeltas+10)
	want := answerBlock{Rows: make([][]string, maxDeltas+10)}
	for idx := range got {
		got[idx] = []any{"got"}
		want.Rows[idx] = []string{"want"}
	}

	result := compareQuery("q1", got, want)
	if len(result.deltas) != maxDeltas {
		t.Fatalf("deltas = %d, want %d", len(result.deltas), maxDeltas)
	}
}
