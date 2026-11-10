package tpcds

import (
	"fmt"
	"strings"
	"testing"
)

func TestGeneratedQuerySetIsComplete(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql"} {
		for _, scale := range []float64{0.01, 1, 100} {
			for _, seed := range []int64{1, 19620718, 99999} {
				for _, stream := range []int{0, 1, 7} {
					t.Run(fmt.Sprintf("%s/sf=%g/seed=%d/stream=%d", dialect, scale, seed, stream), func(t *testing.T) {
						queries, err := generateStream(dialect, scale, seed, stream)
						if err != nil {
							t.Fatal(err)
						}

						if len(queries) != 103 {
							t.Fatalf("got %d statements, want 103", len(queries))
						}

						seen := make(map[string]bool)

						for _, q := range queries {
							name, _, _ := strings.Cut(q.name, "_")

							seen[name] = true
							if q.sql == "" {
								t.Fatalf("empty query %s", q.name)
							}

							if dialect == "mysql" && strings.Contains(strings.ToLower(q.sql), "full outer join") {
								t.Fatalf("unsupported FULL OUTER JOIN in %s", q.name)
							}
						}

						for i := 1; i <= 99; i++ {
							if !seen[fmt.Sprintf("query%d", i)] {
								t.Errorf("query%d missing", i)
							}
						}
					})
				}
			}
		}
	}
}
