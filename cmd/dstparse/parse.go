// Package main in cmd/dstparse parses TPC-DS dsdgen .dst distribution
// files into the uniform Dict-shaped JSON document consumed by the
// relations data generator. This file is the parser; main.go is the CLI
// front-end.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// dict is the JSON shape emitted for each named distribution. The layout
// matches the Dict proto (see datageneration-plan.md §3.2): a list of
// column names, a list of named weight profiles, and parallel
// values/weights per row.
type dict struct {
	Columns    []string  `json:"columns"`
	WeightSets []string  `json:"weight_sets"`
	Rows       []dictRow `json:"rows"`
}

// dictRow is one empirical data point: `Values` parallel to `dict.Columns`,
// `Weights` parallel to `dict.WeightSets`. Empty `Weights` means the row
// belongs to a uniform dict.
type dictRow struct {
	Values  []string `json:"values"`
	Weights []int64  `json:"weights,omitempty"`
}

// doc is the top-level JSON document emitted by dstparse / tpch-dists.
type doc struct {
	Version       string           `json:"version"`
	Source        string           `json:"source"`
	Distributions map[string]*dict `json:"distributions"`
}

// Grammar (subset emitted by real TPC-DS .dst files):
//
//	create <name>;
//	set types = (T1, T2, ...);
//	set weights = N;
//	set names = (c1, c2, ..., cK : w1, w2, ..., wM);   -- optional
//	add (V1, V2, ...: W1, W2, ..., WN);
//	add (V1, V2, ...: ...);
//
// Multiple statements per line separated by `;`. Lines beginning with
// `--`, or trailing `-- ...`, are comments. Strings double-quoted, ints
// bare. Whitespace around commas/colons is insignificant at the top
// level. Block `{ ... }` comments (sometimes appearing in wild .dst)
// are not emitted by current dsdgen but we skip them defensively.
//
// When `set names` is absent the parser synthesises column names
// (`col1`, `col2`, ...) and a single default weight set called
// `default`. When `set weights` is `0` (or absent) the dict is uniform
// and each row's `Weights` slice is empty.

// parseStream reads a whole .dst source from r and returns the
// distributions in declaration order. Errors carry a 1-based line
// number.
func parseStream(r io.Reader) ([]*namedDict, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	p := &parser{}

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		p.line = lineNum

		line := stripLineComment(scanner.Text())
		for _, stmt := range splitTopSemis(line) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if err := p.stmt(stmt); err != nil {
				return nil, fmt.Errorf("dstparse: line %d: %w", lineNum, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("dstparse: scan: %w", err)
	}
	if p.current != nil {
		p.flush()
	}
	return p.out, nil
}

// namedDict carries the parsed distribution plus its declared name and
// the per-dist counts needed to marshal into the uniform Dict shape.
type namedDict struct {
	name       string
	types      []string
	numWeights int
	columns    []string // parsed from `set names` (before the `:`).
	weightSets []string // parsed from `set names` (after the `:`).
	rows       []dictRow
}

type parser struct {
	out     []*namedDict
	current *namedDict
	line    int
}

func (p *parser) stmt(stmt string) error {
	switch {
	case hasPrefixFold(stmt, "create "):
		p.flush()
		name := strings.TrimSpace(stmt[len("create "):])
		if name == "" {
			return errors.New("create: missing distribution name")
		}
		p.current = &namedDict{name: name}
		return nil

	case hasPrefixFold(stmt, "set types"):
		if p.current == nil {
			return errors.New("set types: no active create")
		}
		list, _, err := parseSetList(stmt)
		if err != nil {
			return fmt.Errorf("set types: %w", err)
		}
		p.current.types = list
		return nil

	case hasPrefixFold(stmt, "set weights"):
		if p.current == nil {
			return errors.New("set weights: no active create")
		}
		_, rhs, ok := strings.Cut(stmt, "=")
		if !ok {
			return errors.New("set weights: missing `=`")
		}
		n, err := strconv.Atoi(strings.TrimSpace(rhs))
		if err != nil {
			return fmt.Errorf("set weights: count: %w", err)
		}
		p.current.numWeights = n
		return nil

	case hasPrefixFold(stmt, "set names"):
		if p.current == nil {
			return errors.New("set names: no active create")
		}
		cols, wsets, err := parseSetList(stmt)
		if err != nil {
			return fmt.Errorf("set names: %w", err)
		}
		p.current.columns = cols
		p.current.weightSets = wsets
		return nil

	case hasPrefixFold(stmt, "add "), hasPrefixFold(stmt, "add("):
		if p.current == nil {
			return errors.New("add: no active create")
		}
		row, err := parseAdd(stmt, p.current.numWeights)
		if err != nil {
			return err
		}
		p.current.rows = append(p.current.rows, row)
		return nil

	default:
		return fmt.Errorf("unknown statement %q", firstToken(stmt))
	}
}

func (p *parser) flush() {
	if p.current != nil {
		p.out = append(p.out, p.current)
		p.current = nil
	}
}

// parseSetList splits the parenthesised body of `set X = (...)` on the
// first top-level colon. Tokens before the colon are the "lead" list
// (column names for `set names`, type names for `set types`); tokens
// after are the "tail" list (weight-set names for `set names`). Empty
// tail slice is returned when no colon is present.
func parseSetList(stmt string) (lead, tail []string, err error) {
	open := strings.Index(stmt, "(")
	closeIdx := strings.LastIndex(stmt, ")")
	if open < 0 || closeIdx <= open {
		return nil, nil, errors.New("missing `(...)` body")
	}
	inner := stmt[open+1 : closeIdx]

	if colon := splitOnTopColon(inner); colon >= 0 {
		lead = trimAll(splitTopCommas(inner[:colon]))
		tail = trimAll(splitTopCommas(inner[colon+1:]))
	} else {
		lead = trimAll(splitTopCommas(inner))
	}
	return lead, tail, nil
}

// parseAdd parses `add (V1, V2, ...: W1, W2, ...)` into a dictRow.
// Weight count must equal numWeights when numWeights > 0; otherwise a
// zero-weight row (uniform) is allowed.
func parseAdd(stmt string, numWeights int) (dictRow, error) {
	open := strings.Index(stmt, "(")
	closeIdx := strings.LastIndex(stmt, ")")
	if open < 0 || closeIdx <= open {
		return dictRow{}, errors.New("add: missing `(...)` body")
	}
	inner := stmt[open+1 : closeIdx]

	var valuesPart, weightsPart string
	if colon := splitOnTopColon(inner); colon >= 0 {
		valuesPart = inner[:colon]
		weightsPart = inner[colon+1:]
	} else {
		valuesPart = inner
	}

	values := stripQuotes(trimAll(splitTopCommas(valuesPart)))

	var weights []int64
	if weightsPart != "" {
		for _, w := range trimAll(splitTopCommas(weightsPart)) {
			if w == "" {
				continue
			}
			n, err := strconv.ParseInt(w, 10, 64)
			if err != nil {
				return dictRow{}, fmt.Errorf("add: weight %q: %w", w, err)
			}
			weights = append(weights, n)
		}
	}

	if numWeights > 0 && len(weights) != numWeights {
		return dictRow{}, fmt.Errorf(
			"add: got %d weights, declared `set weights = %d`",
			len(weights), numWeights,
		)
	}

	return dictRow{Values: values, Weights: weights}, nil
}

// toDict materialises the uniform Dict-shaped JSON struct. Synthesises
// default column / weight-set names when the .dst did not declare them.
func (nd *namedDict) toDict() *dict {
	cols := nd.columns
	if len(cols) == 0 {
		// Default: one column per declared type, named col1..colN.
		n := len(nd.types)
		if n == 0 {
			n = 1
		}
		if n == 1 {
			cols = []string{"value"}
		} else {
			cols = make([]string, n)
			for i := range cols {
				cols[i] = fmt.Sprintf("col%d", i+1)
			}
		}
	}

	wsets := nd.weightSets
	if len(wsets) == 0 {
		if nd.numWeights <= 0 {
			wsets = nil
		} else if nd.numWeights == 1 {
			wsets = []string{"default"}
		} else {
			wsets = make([]string, nd.numWeights)
			for i := range wsets {
				wsets[i] = fmt.Sprintf("w%d", i+1)
			}
		}
	}

	rows := make([]dictRow, len(nd.rows))
	copy(rows, nd.rows)

	return &dict{
		Columns:    cols,
		WeightSets: wsets,
		Rows:       rows,
	}
}

// stripLineComment removes a trailing `--` comment (and the newline).
// Honours `"..."` quotes so that `--` inside a string is not treated
// as a comment.
func stripLineComment(line string) string {
	inQuote := false
	for i := 0; i < len(line)-1; i++ {
		if line[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote && line[i] == '-' && line[i+1] == '-' {
			return line[:i]
		}
	}
	return line
}

// splitTopSemis splits a line on `;` outside of `"..."`.
func splitTopSemis(line string) []string {
	var out []string
	var buf strings.Builder
	inQuote := false
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
			buf.WriteRune(r)
		case r == ';' && !inQuote:
			out = append(out, buf.String())
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

// splitOnTopColon returns the byte index of the first `:` not inside
// `"..."`, or -1 if none.
func splitOnTopColon(s string) int {
	inQuote := false
	for i, r := range s {
		if r == '"' {
			inQuote = !inQuote
		}
		if r == ':' && !inQuote {
			return i
		}
	}
	return -1
}

// splitTopCommas splits on `,` outside of `"..."`.
func splitTopCommas(s string) []string {
	var out []string
	var buf strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			buf.WriteRune(r)
		case r == ',' && !inQuote:
			out = append(out, buf.String())
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

func trimAll(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func stripQuotes(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		s = strings.TrimSpace(s)
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			s = s[1 : len(s)-1]
		}
		out[i] = s
	}
	return out
}

func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return strings.EqualFold(s[:len(prefix)], prefix)
}

func firstToken(stmt string) string {
	stmt = strings.TrimSpace(stmt)
	if i := strings.IndexAny(stmt, " \t("); i > 0 {
		return stmt[:i]
	}
	return stmt
}
