package bench

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// twoSectionCorpus is a minimal valid corpus used across the resolver tests.
const twoSectionCorpus = "--+ schema\n--= create\nSELECT 1\n--+ workload\n--= point\nSELECT 2 WHERE id = :id\n"

// newResolverRun builds a Run wired to a Test with the given baked query sets,
// slot kind and env, without exercising the executor stack. The baked sources
// are installed into the Run as [Def.Queries] would do.
func newResolverRun(t *testing.T, kind string, bakes []BakedQuerySet, env map[string]string) *Run {
	t.Helper()
	tst := &Test{Name: "x"}
	r := &Run{
		test:   tst,
		seed:   1,
		slots:  []slotSpec{{name: "main", kind: kind}},
		getenv: envMap(env),
	}
	r.bakes = make(map[string]*BakedQuerySet, len(bakes))
	for i := range bakes {
		r.bakes[bakes[i].Name] = &bakes[i]
	}
	return r
}

func TestQueries_GenericFallback(t *testing.T) {
	r := newResolverRun(t, "pg", []BakedQuerySet{{
		Name:    "tpcc",
		Generic: []byte(twoSectionCorpus),
	}}, nil)

	f, err := r.Queries("tpcc")
	if err != nil {
		t.Fatalf("Queries: %v", err)
	}
	if _, ok := f.Query("workload", "point"); !ok {
		t.Fatal("generic fallback did not resolve the corpus")
	}
	rs := r.qset["tpcc"]
	if rs.source != "baked:tpcc.sql (generic)" {
		t.Fatalf("source = %q, want baked generic", rs.source)
	}
}

func TestQueries_PerKindWins(t *testing.T) {
	r := newResolverRun(t, "ydb", []BakedQuerySet{{
		Name:    "tpcc",
		Generic: []byte("--+ s\n--= g\nSELECT 'generic'\n"),
		PerKind: map[string][]byte{
			"ydb": []byte("--+ s\n--= g\nSELECT 'ydb'\n"),
		},
	}}, nil)

	f, err := r.Queries("tpcc")
	if err != nil {
		t.Fatalf("Queries: %v", err)
	}
	q, _ := f.Query("s", "g")
	if q.Raw != "SELECT 'ydb'" {
		t.Fatalf("per-kind did not win: raw = %q", q.Raw)
	}
	if rs := r.qset["tpcc"]; rs.source != "baked:tpcc.ydb.sql" {
		t.Fatalf("source = %q, want per-kind", rs.source)
	}
}

func TestQueries_OverrideWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my.sql")
	if err := os.WriteFile(path, []byte("--+ s\n--= o\nSELECT 'override'\n"), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}
	r := newResolverRun(t, "pg", []BakedQuerySet{{
		Name:    "tpcc",
		Generic: []byte("--+ s\n--= g\nSELECT 'generic'\n"),
	}}, map[string]string{"STROPPY_QUERIES_TPCC": path})

	f, err := r.Queries("tpcc")
	if err != nil {
		t.Fatalf("Queries: %v", err)
	}
	q, _ := f.Query("s", "o")
	if q.Raw != "SELECT 'override'" {
		t.Fatalf("override did not win: raw = %q", q.Raw)
	}
	if rs := r.qset["tpcc"]; rs.source != "override:"+path {
		t.Fatalf("source = %q, want override:%s", rs.source, path)
	}
}

func TestQueries_OverrideBrokenIsHardError(t *testing.T) {
	r := newResolverRun(t, "pg", []BakedQuerySet{{
		Name:    "tpcc",
		Generic: []byte(twoSectionCorpus),
	}}, map[string]string{"STROPPY_QUERIES_TPCC": "/does/not/exist.sql"})

	if _, err := r.Queries("tpcc"); err == nil {
		t.Fatal("broken override should be a hard error, not a silent fall-through to baked")
	}
}

func TestQueries_MemoizedAndTracked(t *testing.T) {
	r := newResolverRun(t, "pg", []BakedQuerySet{{
		Name:    "a", Generic: []byte(twoSectionCorpus),
	}, {
		Name:    "b", Generic: []byte(twoSectionCorpus),
	}}, nil)

	if _, err := r.Queries("a"); err != nil {
		t.Fatalf("a: %v", err)
	}
	if _, err := r.Queries("a"); err != nil { // second call hits the memo
		t.Fatalf("a (2nd): %v", err)
	}
	if _, err := r.Queries("b"); err != nil {
		t.Fatalf("b: %v", err)
	}
	if got, want := r.qOrder, []string{"a", "b"}; !slices.Equal(got, want) {
		t.Fatalf("qOrder = %v, want %v (no dup on re-ask)", got, want)
	}
}

func TestQueries_MissingSurfacesErrorWithHint(t *testing.T) {
	r := newResolverRun(t, "mysql", []BakedQuerySet{{
		Name: "tpcc", // no Generic, no PerKind
	}}, nil)

	_, err := r.Queries("tpcc")
	if err == nil {
		t.Fatal("expected an error for an unresolved query-set")
	}
	// The error must tell the user the override env var and the baked file
	// names so they know exactly what to provide.
	for _, want := range []string{"STROPPY_QUERIES_TPCC", "tpcc.mysql.sql", "tpcc.sql"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing hint %q", err.Error(), want)
		}
	}
	if rs := r.qset["tpcc"]; rs.source != "missing" {
		t.Fatalf("source = %q, want missing", rs.source)
	}
}

func TestQueryOverrideEnv(t *testing.T) {
	cases := []struct{ in, want string }{
		{"tpcc", "STROPPY_QUERIES_TPCC"},
		{"schema", "STROPPY_QUERIES_SCHEMA"},
	}
	for _, c := range cases {
		if got := queryOverrideEnv(c.in); got != c.want {
			t.Errorf("queryOverrideEnv(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Compile-time: ensure the resolver returns the sqlfile types it claims.
var _ = (*sqlfile.File)(nil)
