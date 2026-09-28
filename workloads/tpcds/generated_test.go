package tpcds

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestGeneratedPostgresQuerySetIsComplete(t *testing.T) {
	for _, scale := range []float64{0.01, 1, 100} {
		for _, seed := range []int64{1, 19620718, 99999} {
			for _, stream := range []int{0, 1, 7} {
				t.Run(fmt.Sprintf("sf=%g/seed=%d/stream=%d", scale, seed, stream), func(t *testing.T) {
					queries, err := generateStream("postgres", scale, seed, stream)
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

func TestGeneratedMySQLCannotSilentlySkipQueries(t *testing.T) {
	queries, err := generateStream("mysql", 0.01, 19620718, 0)
	if !errors.Is(err, errIncompleteQuerySet) || queries != nil {
		t.Fatalf("expected incomplete-set error and no queries, got %v, %d queries", err, len(queries))
	}

	if !strings.Contains(err.Error(), "full outer join") || !strings.Contains(err.Error(), "baked mysql.sql") {
		t.Fatalf("missing actionable skip reason: %v", err)
	}
}
