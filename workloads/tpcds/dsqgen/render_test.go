package dsqgen

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestMySQLCarrierConcatenation(t *testing.T) {
	for _, pair := range [][2]string{{"UPS", "FEDEX"}, {"ORIENTAL", "BOXBUNDLES"}, {"DHL EXPRESS", "O''BRIEN"}} {
		input := fmt.Sprintf("'%s' || ',' || '%s'", pair[0], pair[1])
		want := fmt.Sprintf("concat('%s', ',', '%s')", pair[0], pair[1])
		if got := rewritePipeConcat(input); got != want {
			t.Errorf("rewritePipeConcat(%q) = %q, want %q", input, got, want)
		}
	}
	for _, seed := range []int64{1, 42, 19620718, 99999} {
		result, err := Generate(MySQL, 0.01, seed, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, query := range result.Queries {
			if strings.Contains(query.SQL, "||") {
				t.Errorf("seed %d %s retains logical OR: %s", seed, query.Name, query.SQL)
			}
		}
	}
}

func TestMySQLFullOuterJoinParameters(t *testing.T) {
	for _, name := range []string{"query51", "query97"} {
		t.Run(name, func(t *testing.T) {
			template := loadQuery(t, name)
			for _, month := range []int64{1176, 1200, 1224} {
				env := map[string][]cell{"DMS": {{isInt: true, i: month}}, "_LIMIT": {{isInt: true, i: 17}}}
				sql := strings.ToLower(MySQL.render(template, env, 1, 0, 1))
				if strings.Contains(sql, "full outer join") || strings.Contains(sql, "[dms]") {
					t.Fatalf("unrewritten query: %s", sql)
				}
				if strings.Count(sql, "union all") != 1 || !strings.Contains(sql, "left join") || !strings.Contains(sql, "right join") {
					t.Fatalf("missing full join emulation: %s", sql)
				}
				if strings.Count(sql, fmt.Sprintf("d_month_seq between %d and %d", month, month)) != 2 {
					t.Fatalf("lost month substitution: %s", sql)
				}
				if !strings.Contains(sql, "limit 17") {
					t.Fatalf("lost limit substitution: %s", sql)
				}
			}
		})
	}
}

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
