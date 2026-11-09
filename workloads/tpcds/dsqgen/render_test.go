package dsqgen

import "testing"

// TestPostgresStreamsAliasEveryDerivedTable guards the templates (2, 14, 23, 49) whose
// derived tables carry no alias: PostgreSQL before 16 and Greenplum reject them.
func TestPostgresStreamsAliasEveryDerivedTable(t *testing.T) {
	t.Parallel()

	for stream := range 3 {
		res, err := Generate(Postgres, 1, 19620718, stream)
		if err != nil {
			t.Fatal(err)
		}

		for _, q := range res.Queries {
			if again := aliasDerivedTables(q.SQL); again != q.SQL {
				t.Errorf("stream %d %s has a derived table without an alias", stream, q.Name)
			}
		}
	}
}
