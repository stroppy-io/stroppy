package sqlfile

import "testing"

func parseOneQuery(t *testing.T, sql string) *Query {
	t.Helper()

	f := mustParse(t, "--= q\n"+sql)

	return f.Section("")[0]
}

func TestRewrite_DollarReusesPositionForRepeatedParam(t *testing.T) {
	q := parseOneQuery(t, "SELECT * FROM users WHERE id = :id OR parent_id = :id;")

	if got := q.Params(); !equalStrings(got, []string{"id"}) {
		t.Fatalf("Params = %v", got)
	}

	want := "SELECT * FROM users WHERE id = $1 OR parent_id = $1;"
	if got := q.Text(Dollar); got != want {
		t.Fatalf("Text(Dollar) = %q, want %q", got, want)
	}
}

func TestRewrite_QuestionEmitsOnePlaceholderPerOccurrence(t *testing.T) {
	// mysql's database/sql driver cannot back-reference a placeholder, so a
	// repeated :id must produce a "?" for every occurrence (matching
	// mysqlDialect.Deduplicate() == false in the v5 driver).
	q := parseOneQuery(t, "SELECT * FROM users WHERE id = :id OR parent_id = :id;")

	want := "SELECT * FROM users WHERE id = ? OR parent_id = ?;"
	if got := q.Text(Question); got != want {
		t.Fatalf("Text(Question) = %q, want %q", got, want)
	}
}

func TestRewrite_DollarOrdersByFirstOccurrence(t *testing.T) {
	q := parseOneQuery(t, "INSERT INTO t (a, b, c) VALUES (:c, :a, :b);")

	if got := q.Params(); !equalStrings(got, []string{"c", "a", "b"}) {
		t.Fatalf("Params = %v", got)
	}

	want := "INSERT INTO t (a, b, c) VALUES ($1, $2, $3);"
	if got := q.Text(Dollar); got != want {
		t.Fatalf("Text(Dollar) = %q, want %q", got, want)
	}
}

func TestRewrite_CastSurvives(t *testing.T) {
	q := parseOneQuery(t, "SELECT :date::date, x::integer FROM t WHERE y = :id;")

	if got := q.Params(); !equalStrings(got, []string{"date", "id"}) {
		t.Fatalf("Params = %v, want [date id] (x::integer must not extract \"integer\")", got)
	}

	want := "SELECT $1::date, x::integer FROM t WHERE y = $2;"
	if got := q.Text(Dollar); got != want {
		t.Fatalf("Text(Dollar) = %q, want %q", got, want)
	}
}

func TestRewrite_ParamInSingleQuotedStringUntouched(t *testing.T) {
	q := parseOneQuery(t, "SELECT * FROM t WHERE msg = 'value :id here' AND x = :id;")

	if got := q.Params(); !equalStrings(got, []string{"id"}) {
		t.Fatalf("Params = %v, want just the real :id", got)
	}

	want := "SELECT * FROM t WHERE msg = 'value :id here' AND x = $1;"
	if got := q.Text(Dollar); got != want {
		t.Fatalf("Text(Dollar) = %q, want %q", got, want)
	}
}

func TestRewrite_ParamInBlockCommentUntouched(t *testing.T) {
	q := parseOneQuery(t, "SELECT :id /* not :fake */ FROM t;")

	if got := q.Params(); !equalStrings(got, []string{"id"}) {
		t.Fatalf("Params = %v", got)
	}

	want := "SELECT $1 /* not :fake */ FROM t;"
	if got := q.Text(Dollar); got != want {
		t.Fatalf("Text(Dollar) = %q, want %q", got, want)
	}
}

func TestRewrite_ParamInDollarQuotedBodyUntouched(t *testing.T) {
	q := parseOneQuery(t, "CREATE FUNCTION f() RETURNS void AS $$ BEGIN x := :not_a_param; END; $$ LANGUAGE plpgsql;")

	if got := q.Params(); len(got) != 0 {
		t.Fatalf("Params = %v, want none (body is opaque)", got)
	}

	if got := q.Text(Dollar); got != q.Raw {
		t.Fatalf("Text(Dollar) = %q, want unchanged Raw %q", got, q.Raw)
	}
}

func TestRewrite_AdjacentParamsSeparatedBySingleSpace(t *testing.T) {
	// A regression check on the boundary-peek design: the v5 Go regex
	// (pkg/driver/sqldriver/run_query.go argsRe) consumes the trailing
	// boundary character as part of a match, so ":a :b" with exactly one
	// separating space only recognizes :a. This scanner only peeks the
	// boundary, so both are recognized.
	q := parseOneQuery(t, "SELECT :a :b;")

	if got := q.Params(); !equalStrings(got, []string{"a", "b"}) {
		t.Fatalf("Params = %v, want [a b]", got)
	}
}

func TestRewrite_DoubleColonWithoutNameNotAParam(t *testing.T) {
	q := parseOneQuery(t, "SELECT col::text FROM t;")

	if got := q.Params(); len(got) != 0 {
		t.Fatalf("Params = %v, want none", got)
	}

	if got := q.Text(Dollar); got != q.Raw {
		t.Fatalf("Text(Dollar) = %q, want unchanged", got)
	}
}

func TestRewrite_NoParams(t *testing.T) {
	q := parseOneQuery(t, "SELECT 1;")

	if got := q.Params(); len(got) != 0 {
		t.Fatalf("Params = %v, want none", got)
	}

	if q.Text(Dollar) != q.Raw || q.Text(Question) != q.Raw {
		t.Fatalf("Text() should equal Raw when there are no params")
	}
}

func TestRewrite_UnterminatedString(t *testing.T) {
	_, _, _, err := rewriteParams("SELECT 'unterminated")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestRewrite_UnterminatedBlockComment(t *testing.T) {
	_, _, _, err := rewriteParams("SELECT 1 /* unterminated")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestRewrite_UnterminatedDollarQuote(t *testing.T) {
	_, _, _, err := rewriteParams("CREATE FUNCTION f() AS $$ BEGIN END;")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestRewrite_DoubledQuoteEscape(t *testing.T) {
	q := parseOneQuery(t, "SELECT 'it''s :id' AS x WHERE y = :id;")

	if got := q.Params(); !equalStrings(got, []string{"id"}) {
		t.Fatalf("Params = %v", got)
	}
}

func TestRewrite_QuotedIdentifierUntouched(t *testing.T) {
	q := parseOneQuery(t, `SELECT 1 AS "not :id" WHERE x = :id;`)

	if got := q.Params(); !equalStrings(got, []string{"id"}) {
		t.Fatalf("Params = %v", got)
	}
}
