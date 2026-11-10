package dsqgen

import (
	"reflect"
	"testing"
)

func TestStreamParametersAreDistinctAndReproducible(t *testing.T) {
	for _, dialect := range []Dialect{Postgres, MySQL} {
		t.Run(dialect.Name, func(t *testing.T) {
			first, err := Generate(dialect, 0.01, 19620718, 0)
			if err != nil {
				t.Fatal(err)
			}
			second, err := Generate(dialect, 0.01, 19620718, 1)
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := Generate(dialect, 0.01, 19620718, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(first.Skipped) != 0 || len(second.Skipped) != 0 {
				t.Fatalf("incomplete streams: %v, %v", first.Skipped, second.Skipped)
			}
			if !reflect.DeepEqual(second, repeated) {
				t.Fatal("same seed and stream produced different queries")
			}
			byName := make(map[string]string, len(first.Queries))
			for _, query := range first.Queries {
				byName[query.Name] = query.SQL
			}
			changed := 0
			for _, query := range second.Queries {
				if byName[query.Name] != query.SQL {
					changed++
				}
			}
			if changed == 0 {
				t.Fatal("different streams reused identical query parameters")
			}
		})
	}
}
