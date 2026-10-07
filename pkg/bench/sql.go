package bench

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SQL is a parsed SQL file split into named sections and the named/anonymous
// queries within each. Drivers bind :name parameters, so parsing keeps only
// section/query markers and full-line comment stripping.
type SQL struct {
	sections map[string][]sqlQuery
}

type sqlQuery struct {
	name string
	sql  string
}

const (
	sectionPrefix = "--+"
	queryPrefix   = "--="
	commentPrefix = "--"
)

// ParseSQL parses SQL text into sections (the string form of LoadSQL, for inline SQL).
func ParseSQL(content string) *SQL { return parseSQL(content) }

// QueryFiles loads ordinary workload-owned filesystems with local overrides.
type QueryFiles struct{}

func (QueryFiles) Load(files fs.FS, filename string) (*SQL, error) {
	data, err := readSQLFile(files, filename)
	if err != nil {
		return nil, err
	}

	return parseSQL(string(data)), nil
}

// Override reads only the explicit filename; missing overrides never use embedded data.
func (QueryFiles) Override(filename string) (*SQL, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return parseSQL(string(data)), nil
}

func readSQLFile(files fs.FS, filename string) ([]byte, error) {
	if data, err := os.ReadFile(filename); err == nil {
		return data, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	if data, err := os.ReadFile(filepath.Join(home, ".stroppy", filename)); err == nil {
		return data, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	if files == nil {
		return nil, fmt.Errorf("query file %q: %w", filename, fs.ErrNotExist)
	}

	return fs.ReadFile(files, filename)
}

// QueryHandle preserves immutable query identity alongside its text.
type QueryHandle struct {
	Name string
	Text string
}

func (s *SQL) Lookup(section, name string) (QueryHandle, bool) {
	text, found := s.Query(section, name)

	return QueryHandle{Name: section + "/" + name, Text: text}, found && strings.TrimSpace(text) != ""
}

func (s *SQL) Require(section, name string) QueryHandle {
	q, found := s.Lookup(section, name)
	if !found {
		invalid("query", inputError("missing required query %s/%s", section, name))
	}

	return q
}

func (b *Bench) ExecQuery(ctx context.Context, q QueryHandle, args map[string]any) error {
	return b.Exec(ctx, q.Text, args)
}

func (t *Tx) ExecQuery(ctx context.Context, q QueryHandle, args map[string]any) error {
	return t.Exec(ctx, q.Text, args)
}

func parseSQL(content string) *SQL {
	s := &SQL{sections: map[string][]sqlQuery{}}

	var (
		name  string
		chunk []string
	)

	flush := func() {
		if name == "" && len(chunk) == 0 {
			return
		}

		s.sections[name] = parseQueries(chunk)
	}

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), sectionPrefix) {
			flush()

			name = strings.TrimSpace(strings.TrimPrefix(line, sectionPrefix))
			chunk = nil

			continue
		}

		chunk = append(chunk, line)
	}

	flush()

	return s
}

// parseQueries splits one section into queries. A `--= name` line starts a new
// query (anonymous when name is empty); full-line `--` comments are stripped.
// SQL before the first `--=` (none in current assets) is dropped.
func parseQueries(lines []string) []sqlQuery {
	var queries []sqlQuery

	hasName := false
	name := ""

	body := make([]string, 0, len(lines))

	flush := func() {
		if hasName {
			queries = append(queries, sqlQuery{name: name, sql: strings.TrimSpace(strings.Join(body, "\n"))})
		}
	}

	for _, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, queryPrefix):
			flush()

			name = strings.TrimSpace(strings.TrimPrefix(line, queryPrefix))
			hasName = true
			body = body[:0]
		case strings.HasPrefix(t, commentPrefix):
			// skip full-line comments
		default:
			body = append(body, line)
		}
	}

	flush()

	return queries
}

// Section returns the SQL text of every query in a section (anonymous queries
// included). Empty slice if the section is absent (callers treat missing as no-op).
func (s *SQL) Section(name string) []string {
	qs := s.sections[name]

	out := make([]string, 0, len(qs))
	for _, q := range qs {
		if q.sql != "" {
			out = append(out, q.sql)
		}
	}

	return out
}

// Query returns the SQL text of one named query within a section.
func (s *SQL) Query(section, query string) (string, bool) {
	for _, q := range s.sections[section] {
		if q.name == query {
			return q.sql, true
		}
	}

	return "", false
}

// Names returns the query names of a section in file order. Workloads with a flat
// query file (no sections) keep every query under the empty section name "".
func (s *SQL) Names(section string) []string {
	qs := s.sections[section]

	out := make([]string, 0, len(qs))
	for _, q := range qs {
		if q.sql != "" {
			out = append(out, q.name)
		}
	}

	return out
}
