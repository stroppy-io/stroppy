package dsqgen

import (
	"regexp"
	"testing"
)

// TestPostgresStreamsAliasDerivedTables pins the templates whose derived tables carry no
// alias in the source: PostgreSQL before 16 and Greenplum reject them. It counts the
// aliases in the rendered SQL itself rather than re-running the helper that adds them,
// so a helper that misses a derived table, or does nothing, fails here.
func TestPostgresStreamsAliasDerivedTables(t *testing.T) {
	t.Parallel()

	// Derived tables the rendered query must close with an alias: query 2 has one, 14 and
	// 23 one or two in each of their two parts, 49 one.
	want := map[string]int{"query2": 1, "query14": 2, "query23": 4, "query49": 1}
	aliased := regexp.MustCompile(`\)\s+(dt\d+|x)\b`)

	for stream := range 3 {
		res, err := Generate(Postgres, 1, 19620718, stream)
		if err != nil {
			t.Fatal(err)
		}

		got := map[string]int{}

		for _, q := range res.Queries {
			if _, tracked := want[q.Name]; tracked {
				got[q.Name] = len(aliased.FindAllString(q.SQL, -1))
			}
		}

		for name, atLeast := range want {
			if got[name] < atLeast {
				t.Errorf("stream %d %s has %d aliased derived tables, want at least %d", stream, name, got[name], atLeast)
			}
		}
	}
}
