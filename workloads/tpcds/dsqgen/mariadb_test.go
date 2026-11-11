package dsqgen

import (
	"fmt"
	"strings"
	"testing"
)

func TestMariaDBStreamsAndQualification(t *testing.T) {
	for _, seed := range []int64{1, 19620718, 99999} {
		for stream := range 3 {
			result, err := Generate(MariaDB, 0.01, seed, stream)
			if err != nil {
				t.Fatal(err)
			}
			assertMariaDBQuerySet(t, result)
		}
	}
	result, err := Qualification(MariaDB)
	if err != nil {
		t.Fatal(err)
	}
	assertMariaDBQuerySet(t, result)
	if got := strings.Count(QualificationSQL(MariaDB, result), "--= query_"); got != 103 {
		t.Fatalf("got %d qualification statements, want 103", got)
	}
}

func assertMariaDBQuerySet(t *testing.T, result *Result) {
	t.Helper()
	if len(result.Queries) != 99 || len(result.Skipped) != 0 {
		t.Fatalf("incomplete MariaDB set: queries=%d skipped=%v", len(result.Queries), result.Skipped)
	}
	for _, query := range result.Queries {
		if strings.Contains(strings.ToLower(query.SQL), "grouping(") {
			t.Fatalf("%s retains unsupported GROUPING function", query.Name)
		}
		if query.Name == "query72" && !strings.Contains(query.SQL, "d3.d_date > d1.d_date + interval 5 day") {
			t.Fatalf("query72 lost calendar-date arithmetic: %s", query.SQL)
		}
		for _, statement := range strings.Split(query.SQL, ";") {
			clauses := topLevelClauses(statement)
			group, hasGroup := clauses["group by"]
			order, hasOrder := clauses["order by"]
			if hasGroup && hasOrder && order > group && strings.Contains(statement[group:order], "with rollup") {
				t.Fatalf("%s retains same-block rollup/order", query.Name)
			}
		}
	}
}

func TestMariaDBRollupOrdering(t *testing.T) {
	input := "WITH rows AS (SELECT 'order by' a, 1 v) SELECT a, SUM(v) n FROM rows " +
		"GROUP BY ROLLUP(a) ORDER BY n DESC LIMIT 7; SELECT 1"
	got := mariaDBTransform("query5", input)
	if !strings.HasPrefix(got, "WITH rows AS") ||
		!strings.Contains(got, ") rollup_result\nORDER BY n DESC LIMIT 7") ||
		!strings.HasSuffix(got, "; SELECT 1") {
		t.Fatalf("CTE, outer ordering/limit, or next statement lost: %s", got)
	}
}

func TestMariaDBGroupingRetainsLimit(t *testing.T) {
	for _, name := range []string{"query27", "query36", "query70", "query86"} {
		template := loadQuery(t, name)
		ev := newEvaluator(19620718, 1, newDistCache())
		env, err := ev.evalTemplate(template)
		if err != nil {
			t.Fatal(err)
		}
		env["_LIMIT"] = []cell{intCell(7)}
		sql := MariaDB.render(template, env, 1, 0, 1)
		if !strings.Contains(strings.ToLower(sql), "limit 7") || strings.Contains(sql, "grouping(") {
			t.Fatalf("%s lost substituted limit: %s", name, sql)
		}
		if count := strings.Count(sql, "union all"); count < 2 {
			t.Fatalf("%s has %d grouping branches, want three", name, count+1)
		}
	}
}

func TestTopLevelClauses(t *testing.T) {
	sql := "with x as (select 'from' a) select rank() over (order by a), `order by` " +
		"from x group by a order by a limit 3"
	clauses := topLevelClauses(sql)
	for _, clause := range []string{"select", "from", "group by", "order by", "limit"} {
		position, ok := clauses[clause]
		if !ok || !strings.HasPrefix(sql[position:], clause) {
			t.Fatal(fmt.Sprintf("missing top-level %s: %v", clause, clauses))
		}
	}
	if clauses["order by"] < clauses["group by"] {
		t.Fatal("window ordering mistaken for final ordering")
	}
}
