package bench

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/stroppy-io/stroppy/workloads"
)

// SQL is a parsed SQL file split into named sections and the named/anonymous
// queries within each. Ports the subset of parse_sql.ts the workloads use: the
// node-sql-parser type detection is unused (drivers bind :params themselves), so
// only section (#+) / query (#=) markers and full-line comment stripping remain.
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

// LoadSQL reads a workload SQL file (cwd → workloads/<preset>/ → embedded) and
// parses it into sections.
func LoadSQL(preset, filename string) (*SQL, error) {
	data, err := readSQLFile(preset, filename)
	if err != nil {
		return nil, err
	}
	return parseSQL(string(data)), nil
}

func readSQLFile(preset, filename string) ([]byte, error) {
	// cwd override first (edit-run loop), mirroring the runner's resolution.
	for _, p := range []string{filename, path.Join("workloads", preset, filename)} {
		if b, err := os.ReadFile(p); err == nil {
			return b, nil
		}
	}
	b, err := workloads.ReadPresetFile(preset, filename)
	if err != nil {
		return nil, fmt.Errorf("load %s/%s: %w", preset, filename, err)
	}
	return b, nil
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
// SQL before the first `--=` (none in practice) is dropped, matching parse_sql.ts.
func parseQueries(lines []string) []sqlQuery {
	var queries []sqlQuery
	hasName := false
	name := ""
	var body []string
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
			body = nil
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
