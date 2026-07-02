package sqlfile

import (
	"strings"
	"testing"
)

// mustParse parses src and fails the test on error.
func mustParse(t *testing.T, src string) *File {
	t.Helper()

	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	return f
}

func names(qs []*Query) []string {
	out := make([]string, len(qs))
	for i, q := range qs {
		out[i] = q.Name
	}

	return out
}

// --- ported from internal/static/tests/parse_sql.test.ts: parse_sql_with_sections ---

func TestSections_GroupsOfQueries(t *testing.T) {
	src := `--+ group one
--= some query
Select 1;
--= some other
Select 2;
--+ group two
--+ group three
-- its empty`

	f := mustParse(t, src)

	if got := f.SectionNames(); !equalStrings(got, []string{"group one", "group two", "group three"}) {
		t.Fatalf("SectionNames = %v", got)
	}

	one := f.Section("group one")
	if got := names(one); !equalStrings(got, []string{"some query", "some other"}) {
		t.Fatalf("group one names = %v", got)
	}

	if one[0].Raw != "Select 1;" || one[1].Raw != "Select 2;" {
		t.Fatalf("group one raw = %q, %q", one[0].Raw, one[1].Raw)
	}

	if got := f.Section("group two"); len(got) != 0 {
		t.Fatalf("group two = %v, want empty", got)
	}

	if got := f.Section("group three"); len(got) != 0 {
		t.Fatalf("group three = %v, want empty", got)
	}
}

func TestSections_AccessBySectionName(t *testing.T) {
	src := `--+ setup
--= create_table
CREATE TABLE users (id INTEGER PRIMARY KEY);
--+ queries
--= select_all
SELECT * FROM users;`

	f := mustParse(t, src)

	setup := f.Section("setup")
	if len(setup) != 1 || setup[0].Name != "create_table" ||
		setup[0].Raw != "CREATE TABLE users (id INTEGER PRIMARY KEY);" {
		t.Fatalf("setup = %+v", setup)
	}
}

func TestSections_AccessBySectionAndQueryName(t *testing.T) {
	src := `--+ setup
--= create_table
CREATE TABLE users (id INTEGER PRIMARY KEY);
--= create_index
CREATE INDEX idx ON users(id);`

	f := mustParse(t, src)

	q, ok := f.Query("setup", "create_table")
	if !ok || q.Raw != "CREATE TABLE users (id INTEGER PRIMARY KEY);" {
		t.Fatalf("Query(setup, create_table) = %+v, %v", q, ok)
	}

	if _, ok := f.Query("setup", "nonexistent"); ok {
		t.Fatalf("Query(setup, nonexistent) found, want not found")
	}
}

// --- ported from internal/static/tests/parse_sql.test.ts: parse_sql (flat, no sections) ---

func TestFlat_SingleQuery(t *testing.T) {
	src := `--= create_table
CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  name TEXT
);`

	f := mustParse(t, src)

	all := f.Section("")
	want := "CREATE TABLE users (\n  id INTEGER PRIMARY KEY,\n  name TEXT\n);"

	if len(all) != 1 || all[0].Name != "create_table" || all[0].Raw != want {
		t.Fatalf("got %+v", all)
	}
}

func TestFlat_MultipleQueries(t *testing.T) {
	src := `--= create_table
CREATE TABLE users (
  id INTEGER PRIMARY KEY
);

--= insert_data
INSERT INTO users (id, name) VALUES (1, 'Alice');

--= select_data
SELECT * FROM users;`

	f := mustParse(t, src)

	all := f.Section("")
	if got := names(all); !equalStrings(got, []string{"create_table", "insert_data", "select_data"}) {
		t.Fatalf("names = %v", got)
	}
}

func TestFlat_AccessByName(t *testing.T) {
	src := `--= create_table
CREATE TABLE users (id INTEGER PRIMARY KEY);
--= select_all
SELECT * FROM users;`

	f := mustParse(t, src)

	q, ok := f.Find("select_all")
	if !ok || q.Raw != "SELECT * FROM users;" {
		t.Fatalf("Find(select_all) = %+v, %v", q, ok)
	}

	if _, ok := f.Find("nonexistent"); ok {
		t.Fatalf("Find(nonexistent) found, want not found")
	}
}

func TestFlat_SkipsCommentLines(t *testing.T) {
	src := `--= create_table
-- This is a comment
CREATE TABLE users (
  id INTEGER PRIMARY KEY
);
-- Another comment
--= insert_data
-- Yet another comment
INSERT INTO users VALUES (1);`

	f := mustParse(t, src)

	all := f.Section("")
	if all[0].Raw != "CREATE TABLE users (\n  id INTEGER PRIMARY KEY\n);" {
		t.Fatalf("create_table raw = %q", all[0].Raw)
	}

	if all[1].Raw != "INSERT INTO users VALUES (1);" {
		t.Fatalf("insert_data raw = %q", all[1].Raw)
	}
}

func TestEmptyContent(t *testing.T) {
	f := mustParse(t, "")
	if got := f.SectionNames(); len(got) != 0 {
		t.Fatalf("SectionNames = %v, want empty", got)
	}
}

func TestOnlyComments(t *testing.T) {
	src := "-- This is just a comment\n-- Another comment"

	f := mustParse(t, src)
	if got := f.SectionNames(); len(got) != 0 {
		t.Fatalf("SectionNames = %v, want empty", got)
	}
}

func TestQueryNameWithoutSQL(t *testing.T) {
	src := `--= empty_query
--= another_query
SELECT 1;`

	f := mustParse(t, src)

	all := f.Section("")
	if len(all) != 2 || all[0].Name != "empty_query" || all[0].Raw != "" {
		t.Fatalf("empty_query = %+v", all[0])
	}

	if all[1].Name != "another_query" || all[1].Raw != "SELECT 1;" {
		t.Fatalf("another_query = %+v", all[1])
	}
}

func TestMultilineSQL(t *testing.T) {
	src := `--= complex_query
SELECT
  u.id,
  u.name,
  COUNT(o.id) as order_count
FROM users u
LEFT JOIN orders o ON u.id = o.user_id
GROUP BY u.id, u.name
HAVING COUNT(o.id) > 5;`

	f := mustParse(t, src)

	want := "SELECT\n  u.id,\n  u.name,\n  COUNT(o.id) as order_count\n" +
		"FROM users u\nLEFT JOIN orders o ON u.id = o.user_id\n" +
		"GROUP BY u.id, u.name\nHAVING COUNT(o.id) > 5;"

	if got := f.Section("")[0].Raw; got != want {
		t.Fatalf("Raw = %q, want %q", got, want)
	}
}

func TestEmptyLinesInSQL(t *testing.T) {
	src := `--= create_table
CREATE TABLE users (
  id INTEGER PRIMARY KEY
);


--= insert_data
INSERT INTO users VALUES (1);`

	f := mustParse(t, src)

	all := f.Section("")
	if all[0].Raw != "CREATE TABLE users (\n  id INTEGER PRIMARY KEY\n);" {
		t.Fatalf("create_table raw = %q", all[0].Raw)
	}

	if all[1].Raw != "INSERT INTO users VALUES (1);" {
		t.Fatalf("insert_data raw = %q", all[1].Raw)
	}
}

func TestExtractParameters(t *testing.T) {
	src := `--= insert_with_params
INSERT INTO users (id, name, email) VALUES (:id, :name, :email);

--= select_with_params
SELECT * FROM users WHERE id = :id AND status = :status;`

	f := mustParse(t, src)

	all := f.Section("")
	if got := all[0].Params(); !equalStrings(got, []string{"id", "name", "email"}) {
		t.Fatalf("insert params = %v", got)
	}

	if got := all[1].Params(); !equalStrings(got, []string{"id", "status"}) {
		t.Fatalf("select params = %v", got)
	}
}

func TestDuplicateParametersOnlyIncludedOnce(t *testing.T) {
	src := `--= query_with_duplicates
SELECT * FROM users WHERE id = :id OR parent_id = :id;`

	f := mustParse(t, src)

	if got := f.Section("")[0].Params(); !equalStrings(got, []string{"id"}) {
		t.Fatalf("params = %v", got)
	}
}

func TestParameterAtEndOfLine(t *testing.T) {
	src := `--= query_end_param
SELECT * FROM users WHERE id = :id`

	f := mustParse(t, src)

	q := f.Section("")[0]
	if got := q.Params(); !equalStrings(got, []string{"id"}) {
		t.Fatalf("params = %v", got)
	}
}

func TestParametersWithUnderscores(t *testing.T) {
	src := `--= query_underscore
SELECT * FROM users WHERE user_id = :user_id AND account_name = :account_name;`

	f := mustParse(t, src)

	if got := f.Section("")[0].Params(); !equalStrings(got, []string{"user_id", "account_name"}) {
		t.Fatalf("params = %v", got)
	}
}

func TestQueryNameWithTrailingSpaces(t *testing.T) {
	src := "--= query_name  \nSELECT 1;"

	f := mustParse(t, src)
	if got := f.Section("")[0].Name; got != "query_name" {
		t.Fatalf("Name = %q", got)
	}
}

// --- edge cases beyond the TS suite ---

func TestAnalyzeSectionEmptyQueryName(t *testing.T) {
	// Mirrors workloads/tpcc/pg.sql's "--+ analyze" section: "--=" with no
	// name text still produces one query, named "".
	src := "--+ analyze\n-- comment\n--=\nANALYZE;"

	f := mustParse(t, src)

	qs := f.Section("analyze")
	if len(qs) != 1 || qs[0].Name != "" || qs[0].Raw != "ANALYZE;" {
		t.Fatalf("analyze section = %+v", qs)
	}
}

func TestNoSectionHeadersFallsBackToEmptyName(t *testing.T) {
	src := "--= q\nSELECT 1;"

	f := mustParse(t, src)
	if got := f.SectionNames(); !equalStrings(got, []string{""}) {
		t.Fatalf("SectionNames = %v", got)
	}
}

func TestLineNumbers(t *testing.T) {
	src := "--+ s\n--= a\nSELECT 1;\n--= b\nSELECT 2;"

	f := mustParse(t, src)
	qs := f.Section("s")

	if qs[0].Line != 2 || qs[1].Line != 4 {
		t.Fatalf("lines = %d, %d", qs[0].Line, qs[1].Line)
	}
}

func TestSectionsAcrossFileCollideOnFind(t *testing.T) {
	// Mirrors tpcc/pg.sql: "warehouse" is declared under both create_schema
	// and set_unlogged. Find returns the first, in source order.
	src := `--+ create_schema
--= warehouse
CREATE TABLE warehouse (w_id INTEGER);
--+ set_unlogged
--= warehouse
ALTER TABLE warehouse SET UNLOGGED;`

	f := mustParse(t, src)

	q, ok := f.Find("warehouse")
	if !ok || q.Section != "create_schema" {
		t.Fatalf("Find(warehouse) = %+v, %v", q, ok)
	}

	q2, ok := f.Query("set_unlogged", "warehouse")
	if !ok || q2.Section != "set_unlogged" {
		t.Fatalf("Query(set_unlogged, warehouse) = %+v, %v", q2, ok)
	}
}

func TestSemicolonsNotSplit(t *testing.T) {
	// A "--=" entry is one blob regardless of how many top-level ";" it
	// contains — matching v5, which never splits a query body (see
	// create_procedures in the real corpus).
	src := "--= drop\nDROP TABLE a; DROP TABLE b; DROP TABLE c;"

	f := mustParse(t, src)

	q := f.Section("")[0]
	if q.Raw != "DROP TABLE a; DROP TABLE b; DROP TABLE c;" {
		t.Fatalf("Raw = %q", q.Raw)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestParseErrorHasLine(t *testing.T) {
	src := "--+ s\n--= bad\nSELECT '/* unterminated string"

	_, err := Parse([]byte(src))
	if err == nil {
		t.Fatal("Parse: want error")
	}

	var pe *ParseError
	if !asParseError(err, &pe) {
		t.Fatalf("error is not *ParseError: %v", err)
	}

	if pe.Line != 3 {
		t.Fatalf("Line = %d, want 3", pe.Line)
	}

	if !strings.Contains(pe.Error(), "line 3") {
		t.Fatalf("Error() = %q, want it to mention the line", pe.Error())
	}
}

func asParseError(err error, target **ParseError) bool {
	pe, ok := err.(*ParseError) //nolint:errorlint // test helper, single-level assertion is fine
	if !ok {
		return false
	}

	*target = pe

	return true
}
