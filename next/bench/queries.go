package bench

import (
	"fmt"
	"os"
	"strings"

	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// BakedQuerySet declares one query-set a test bakes in: a kind-neutral source
// (the reference dialect, used when no per-kind source matches the active kind)
// and optional per-kind overrides. The SDK resolves the effective source per
// active driver kind at plan time (see [Run.Queries]), honoring a user-provided
// override file above either baked form.
//
// The convention mirrors the override names a user drops in: for query-set
// "<name>", the SDK resolves (highest first) a user file pointed at by env var
// STROPPY_QUERIES_<NAME>, then a baked per-kind source, then the baked generic
// source. A test that ships only a reference dialect registers it as Generic
// and it serves every kind until a per-kind source or user override replaces
// it; a test that ships per-dialect files registers them as PerKind and may
// omit Generic entirely.
type BakedQuerySet struct {
	// Name is the query-set name a test asks for through [Run.Queries] (e.g.
	// "tpcc"). Resolution is keyed on it.
	Name string
	// Generic is the kind-neutral baked source — the reference dialect used
	// when no per-kind source matches. May be nil when PerKind covers every
	// kind the test runs against.
	Generic []byte
	// PerKind maps driver kind to a kind-specific baked source. Wins over
	// Generic for the matching kind. Keys are driver kinds ("pg", "ydb", ...).
	PerKind map[string][]byte
}

// resolvedQuerySet records one [Run.Queries] resolution: the parsed corpus,
// where it came from, and the failure (if any) so probe can surface it without
// re-running resolution.
type resolvedQuerySet struct {
	name   string
	file   *sqlfile.File
	source string // provenance: "override:<path>" | "baked:<name>.<kind>.sql" | "baked:<name>.sql (generic)" | "missing"
	err    error  // parse or read failure of the resolved source; nil on success
}

// Queries resolves query-set name for the run's default driver slot kind and
// returns its parsed corpus. Resolution order, highest priority first:
//
//  1. User-provided override file at the path in env var STROPPY_QUERIES_<NAME>
//     (upper-cased name).
//  2. Baked per-kind source for the active kind ([BakedQuerySet.PerKind]).
//  3. Baked generic source ([BakedQuerySet.Generic]).
//
// Every call is memoized, and the SDK records each name asked for here so probe
// can list the required set back to the user (which files to create to
// override, which dialect is active). The returned [sqlfile.File] is the
// author-facing handle; placeholder rendering ($1 vs ?) is the driver's
// concern, never the caller's.
//
// A missing source or a parse failure is returned as an error; the typical
// caller in a Build callback treats it as fatal (log.Fatalf) since the run
// cannot proceed without the corpus.
func (r *Run) Queries(name string) (*sqlfile.File, error) {
	rs := r.resolveQuerySet(name)
	if rs.err != nil {
		return nil, fmt.Errorf("query-set %q (kind=%q): %w", name, r.activeKind(), rs.err)
	}
	return rs.file, nil
}

// resolveQuerySet finds (or computes and memoizes) the resolution for name.
// It is the single place that walks the override -> per-kind -> generic chain.
func (r *Run) resolveQuerySet(name string) *resolvedQuerySet {
	if r.qset == nil {
		r.qset = make(map[string]*resolvedQuerySet)
	}
	if rs, ok := r.qset[name]; ok {
		return rs
	}

	rs := &resolvedQuerySet{name: name}
	r.qset[name] = rs
	r.qOrder = append(r.qOrder, name)

	envKey := queryOverrideEnv(name)
	var bake *BakedQuerySet
	for i := range r.test.QuerySets {
		if r.test.QuerySets[i].Name == name {
			bake = &r.test.QuerySets[i]
			break
		}
	}
	kind := r.activeKind()

	// 1. User-provided override file (env), highest priority. A set-but-broken
	// override is a hard error: don't fall through to baked silently.
	if r.getenv != nil {
		if path := r.getenv(envKey); path != "" {
			rs.source = "override:" + path
			src, err := os.ReadFile(path)
			if err != nil {
				rs.err = fmt.Errorf("read override %s: %w", path, err)
				return rs
			}
			f, err := sqlfile.Parse(src)
			rs.file, rs.err = f, err
			return rs
		}
	}

	// 2. Baked per-kind source for the active kind.
	if bake != nil && kind != "" {
		if src := bake.PerKind[kind]; len(src) > 0 {
			rs.source = fmt.Sprintf("baked:%s.%s.sql", name, kind)
			f, err := sqlfile.Parse(src)
			rs.file, rs.err = f, err
			return rs
		}
	}

	// 3. Baked generic source.
	if bake != nil && len(bake.Generic) > 0 {
		rs.source = fmt.Sprintf("baked:%s.sql (generic)", name)
		f, err := sqlfile.Parse(bake.Generic)
		rs.file, rs.err = f, err
		return rs
	}

	// Nothing resolved. Tell the user exactly what to provide.
	rs.source = "missing"
	kindHint := ""
	if kind != "" {
		kindHint = fmt.Sprintf(" or bake %s.%s.sql", name, kind)
	}
	rs.err = fmt.Errorf("no source resolved; set %s=<file.sql>%s, or bake %s.sql",
		envKey, kindHint, name)
	return rs
}

// activeKind returns the default driver slot's resolved kind, or "" when no
// slot is configured (probe with no drivers declared).
func (r *Run) activeKind() string {
	if len(r.slots) == 0 {
		return ""
	}
	return r.slots[0].kind
}

// queryOverrideEnv is the env var name a user sets to override query-set name
// with a file on disk: STROPPY_QUERIES_<NAME>, NAME upper-cased.
func queryOverrideEnv(name string) string {
	return "STROPPY_QUERIES_" + strings.ToUpper(name)
}
