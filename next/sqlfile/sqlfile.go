package sqlfile

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	sectionPrefix = "--+"
	queryPrefix   = "--="
	commentPrefix = "--"
)

// Query is one "--= name" entry: its section, source line, raw text and
// named parameters. The author-facing form is Raw + the ":name" markers; the
// per-PlaceholderStyle rewrites are computed once at parse time and read only
// by the dbdrv at Prepare time (see Text).
type Query struct {
	// Section is the enclosing "--+" section name.
	Section string
	// Name is the text following "--=" on its marker line, trimmed.
	Name string
	// Line is the 1-based source line of the "--=" marker.
	Line int
	// Raw is the query text: line comments stripped, otherwise verbatim,
	// with leading/trailing blank lines trimmed.
	Raw string

	params   []string
	occur    []string // per-occurrence param names (with dups) — Question-order
	dollar   string
	question string
}

// Params returns the query's named parameters (":name" markers), in
// first-occurrence order with duplicates removed. The returned slice is
// owned by the Query and must not be mutated.
func (q *Query) Params() []string {
	return q.params
}

// Occurrences returns the query's named parameters in occurrence order — every
// ":name" marker as it appears left to right, duplicates included. It is the
// Question-order counterpart of [Query.Params]: a driver whose placeholder
// syntax emits one marker per occurrence (mysql "?") binds one value per entry
// here, whereas [Params] gives the de-duplicated order a $n-back-reference
// dialect (pgx) binds. The returned slice is owned by the Query.
func (q *Query) Occurrences() []string {
	return q.occur
}

// Text returns Raw with every ":name" marker rewritten to the positional
// placeholder syntax of style. Dollar style reuses one placeholder per
// distinct name ($n back-reference, matching pgx); Question style emits a
// fresh "?" per occurrence, matching database/sql drivers that require one
// bound value per placeholder.
//
// Text is dbdrv-internal: a driver calls it once at Prepare time to select
// its dialect's rendering. Test/author code never calls Text — it passes the
// Query to [driver.Conn].Prepare and the driver picks the style.
func (q *Query) Text(style PlaceholderStyle) string {
	if style == Question {
		return q.question
	}

	return q.dollar
}

// Section is a named, ordered group of queries.
type Section struct {
	Name    string
	Queries []*Query
}

// ParseError reports a parse failure and the source line it occurred on.
type ParseError struct {
	// Line is the 1-based source line the failure is attributed to.
	Line int
	// Err is the underlying cause.
	Err error
}

// Error implements the error interface.
func (e *ParseError) Error() string {
	return fmt.Sprintf("sqlfile: line %d: %v", e.Line, e.Err)
}

// Unwrap exposes the underlying cause for errors.Is/errors.As.
func (e *ParseError) Unwrap() error {
	return e.Err
}

// File is a parsed SQL corpus: sections in source order plus a flat query
// index for lookup by name alone.
type File struct {
	order    []string
	sections map[string][]*Query
	flat     []*Query
}

// SectionNames returns section names in source order.
func (f *File) SectionNames() []string {
	return f.order
}

// Sections returns every section in source order.
func (f *File) Sections() []Section {
	out := make([]Section, len(f.order))
	for i, name := range f.order {
		out[i] = Section{Name: name, Queries: f.sections[name]}
	}

	return out
}

// Section returns the queries of the named section, in source order. It
// returns nil if the section does not exist.
func (f *File) Section(name string) []*Query {
	return f.sections[name]
}

// Query returns the first query named name within section, in source order.
func (f *File) Query(section, name string) (*Query, bool) {
	for _, q := range f.sections[section] {
		if q.Name == name {
			return q, true
		}
	}

	return nil, false
}

// Find returns the first query named name across every section, in source
// order. Section names are not unique across a file (e.g. tpcc's
// create_schema and set_unlogged both declare "--= warehouse"); callers
// that care about a specific section must use Query instead.
func (f *File) Find(name string) (*Query, bool) {
	for _, q := range f.flat {
		if q.Name == name {
			return q, true
		}
	}

	return nil, false
}

// Parse parses the "--+ section" / "--= query" format described in the
// package doc. It never fails on the section/query line grammar itself
// (that grammar has no invalid input, mirroring parse_sql.ts); errors are
// only raised for malformed query bodies the parameter scanner cannot
// safely walk (unterminated string/identifier quoting, block comment or
// "$$" body).
func Parse(src []byte) (*File, error) {
	lines := strings.Split(string(src), "\n")

	f := &File{sections: make(map[string][]*Query)}

	var (
		cur      []string
		curStart = 1
		name     string
		named    bool // true once a real (possibly empty) section name has been committed
	)

	store := func(sectionName string, queries []*Query) {
		if _, exists := f.sections[sectionName]; !exists {
			f.order = append(f.order, sectionName)
		}

		f.sections[sectionName] = queries
		f.flat = append(f.flat, queries...)
	}

	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)

		if !strings.HasPrefix(trimmed, sectionPrefix) {
			cur = append(cur, line)
			continue
		}

		queries, err := parseQueries(name, curStart, cur)
		if err != nil {
			return nil, err
		}

		header := strings.TrimSpace(strings.TrimPrefix(trimmed, sectionPrefix))

		if !named && len(queries) == 0 {
			// Leading content before the first "--+" (blank lines, a file
			// banner comment) never contains a "--=" query, so nothing was
			// lost; adopt this header as the first section name instead of
			// recording a spurious empty-named section.
			name = header
			named = true
			cur = nil
			curStart = lineNo + 1

			continue
		}

		store(name, queries)

		name = header
		named = true
		cur = nil
		curStart = lineNo + 1
	}

	queries, err := parseQueries(name, curStart, cur)
	if err != nil {
		return nil, err
	}

	if !named && len(queries) == 0 {
		return f, nil
	}

	store(name, queries)

	return f, nil
}

// parseQueries splits one section's raw lines into queries: "--= name"
// starts a new query, "--" line comments are dropped, everything else is
// accumulated as the current query's body. lineStart is the 1-based source
// line of lines[0], used to attribute Query.Line and parse errors.
func parseQueries(section string, lineStart int, lines []string) ([]*Query, error) {
	var (
		out      []*Query
		name     string
		nameLine int
		started  bool
		body     []string
	)

	flush := func() error {
		if !started {
			return nil
		}

		text := strings.TrimSpace(strings.Join(body, "\n"))

		q, err := newQuery(section, name, nameLine, text)
		if err != nil {
			return err
		}

		out = append(out, q)

		return nil
	}

	for i, raw := range lines {
		lineNo := lineStart + i
		line := strings.TrimRightFunc(raw, unicode.IsSpace)
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, queryPrefix):
			if err := flush(); err != nil {
				return nil, err
			}

			name = strings.TrimLeftFunc(strings.Replace(line, queryPrefix, "", 1), unicode.IsSpace)
			nameLine = lineNo
			started = true
			body = nil
		case strings.HasPrefix(trimmed, commentPrefix):
			continue
		default:
			body = append(body, line)
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return out, nil
}

func newQuery(section, name string, line int, text string) (*Query, error) {
	params, occur, dollar, question, err := rewriteParams(text)
	if err != nil {
		var se *scanError
		if errors.As(err, &se) {
			// Best-effort line: the "--=" marker line plus newlines consumed
			// before the failure. Leading blank lines trimmed off text by
			// parseQueries make this approximate, not exact.
			return nil, &ParseError{
				Line: line + 1 + strings.Count(text[:se.offset], "\n"),
				Err:  fmt.Errorf("query %q: %w", name, se.err),
			}
		}

		return nil, &ParseError{Line: line, Err: fmt.Errorf("query %q: %w", name, err)}
	}

	return &Query{
		Section:  section,
		Name:     name,
		Line:     line,
		Raw:      text,
		params:   params,
		occur:    occur,
		dollar:   dollar,
		question: question,
	}, nil
}
