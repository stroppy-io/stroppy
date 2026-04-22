// Package main in cmd/tpch-dists parses upstream TPC-H `dists.dss` into
// the uniform Dict-shaped JSON document.
//
// Grammar (case-insensitive keywords, `#` line comments, `|`-separated
// payload):
//
//	BEGIN <name>                   -- start of block
//	COUNT|<n>                      -- declared row count (informational)
//	<value>|<weight>               -- data row
//	... more value/weight pairs ...
//	END <name>                     -- end of block
//
// Values are bare strings (no quoting rule). Weights are non-negative
// integers with one exception: the `nations` dist contains negative
// offsets used by qgen, which we accept as int64. Multiple blocks per
// file; blocks may be separated by `###` banner comments.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// dict / dictRow / doc are re-declared here (not shared across tools)
// to keep each cmd self-contained per the Stage A5 file layout.
type dict struct {
	Columns    []string  `json:"columns"`
	WeightSets []string  `json:"weight_sets"`
	Rows       []dictRow `json:"rows"`
}

type dictRow struct {
	Values  []string `json:"values"`
	Weights []int64  `json:"weights,omitempty"`
}

type doc struct {
	Version       string           `json:"version"`
	Source        string           `json:"source"`
	Distributions map[string]*dict `json:"distributions"`
}

// block is a mutable parse state holding the current BEGIN...END block.
type block struct {
	name     string
	declared int // from COUNT|n; informational, used to validate row count
	rows     []dictRow
}

// maxScannerBuf bounds the bufio.Scanner buffer used when reading
// dists.dss line-by-line.
const maxScannerBuf = 1 << 20

// pipePartsExpected is the number of fields a `<left>|<right>` data line
// must split into.
const pipePartsExpected = 2

// errParse is the sentinel wrapped by every structural parse error.
var errParse = errors.New("parse error")

// streamState is the aggregate parse state threaded through line handlers.
type streamState struct {
	out   map[string]*dict
	order []string
	cur   *block
}

// parseStream reads a whole dists.dss source from r and returns the
// distributions in declaration order.
func parseStream(r io.Reader) (dists map[string]*dict, order []string, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, maxScannerBuf), maxScannerBuf)

	st := &streamState{out: map[string]*dict{}}

	lineNum := 0
	for scanner.Scan() {
		lineNum++

		line := strings.TrimSpace(stripHashComment(scanner.Text()))
		if line == "" {
			continue
		}

		if err := st.handleLine(line, lineNum); err != nil {
			return nil, nil, err
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("tpch-dists: scan: %w", err)
	}

	if st.cur != nil {
		return nil, nil, fmt.Errorf("%w: tpch-dists: unterminated block %q", errParse, st.cur.name)
	}

	return st.out, st.order, nil
}

// handleLine routes one non-empty, de-commented line to the appropriate
// block-level handler, mutating st in place.
func (st *streamState) handleLine(line string, lineNum int) error {
	lower := strings.ToLower(line)

	switch {
	case strings.HasPrefix(lower, "begin "):
		return st.handleBegin(line, lineNum)
	case strings.HasPrefix(lower, "end "):
		return st.handleEnd(line, lineNum)
	default:
		return st.handleData(line, lineNum)
	}
}

// handleBegin opens a new block, rejecting nested BEGINs.
func (st *streamState) handleBegin(line string, lineNum int) error {
	if st.cur != nil {
		return fmt.Errorf(
			"%w: tpch-dists: line %d: BEGIN %q while %q still open",
			errParse, lineNum, line[len("BEGIN "):], st.cur.name,
		)
	}

	name := strings.TrimSpace(line[len("begin "):])
	if name == "" {
		return fmt.Errorf("%w: tpch-dists: line %d: BEGIN missing name", errParse, lineNum)
	}

	st.cur = &block{name: name}

	return nil
}

// handleEnd closes the current block, validates its COUNT, and commits
// the materialized dict into st.out.
func (st *streamState) handleEnd(line string, lineNum int) error {
	if st.cur == nil {
		return fmt.Errorf("%w: tpch-dists: line %d: END with no matching BEGIN", errParse, lineNum)
	}

	name := strings.TrimSpace(line[len("end "):])
	if !strings.EqualFold(name, st.cur.name) {
		return fmt.Errorf(
			"%w: tpch-dists: line %d: END %q does not match BEGIN %q",
			errParse, lineNum, name, st.cur.name,
		)
	}

	if st.cur.declared > 0 && st.cur.declared != len(st.cur.rows) {
		return fmt.Errorf(
			"%w: tpch-dists: line %d: block %q declared COUNT=%d but has %d rows",
			errParse, lineNum, st.cur.name, st.cur.declared, len(st.cur.rows),
		)
	}

	if _, dup := st.out[st.cur.name]; dup {
		return fmt.Errorf("%w: tpch-dists: line %d: duplicate dist %q", errParse, lineNum, st.cur.name)
	}

	st.out[st.cur.name] = blockToDict(st.cur)
	st.order = append(st.order, st.cur.name)
	st.cur = nil

	return nil
}

// handleData processes a non-BEGIN/END data line within the current block.
func (st *streamState) handleData(line string, lineNum int) error {
	if st.cur == nil {
		return fmt.Errorf(
			"%w: tpch-dists: line %d: data line outside BEGIN/END: %q",
			errParse, lineNum, line,
		)
	}

	if err := parseDataLine(line, st.cur); err != nil {
		return fmt.Errorf("tpch-dists: line %d: %w", lineNum, err)
	}

	return nil
}

// parseDataLine handles either `COUNT|N` or `<value>|<weight>`.
func parseDataLine(line string, cur *block) error {
	parts := strings.SplitN(line, "|", pipePartsExpected)
	if len(parts) != pipePartsExpected {
		return fmt.Errorf("%w: expected `a|b`, got %q", errParse, line)
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	if strings.EqualFold(left, "count") {
		n, err := strconv.Atoi(right)
		if err != nil {
			return fmt.Errorf("COUNT value: %w", err)
		}

		if cur.declared > 0 {
			return fmt.Errorf("%w: duplicate COUNT in block", errParse)
		}

		cur.declared = n

		return nil
	}

	weight, err := strconv.ParseInt(right, 10, 64)
	if err != nil {
		return fmt.Errorf("weight %q: %w", right, err)
	}

	cur.rows = append(cur.rows, dictRow{
		Values:  []string{left},
		Weights: []int64{weight},
	})

	return nil
}

// blockToDict materializes the uniform Dict-shaped JSON.
func blockToDict(b *block) *dict {
	rows := make([]dictRow, len(b.rows))
	copy(rows, b.rows)

	return &dict{
		Columns:    []string{"value"},
		WeightSets: []string{"default"},
		Rows:       rows,
	}
}

// stripHashComment removes `#...` trailing comments (entire line if it
// starts with `#`). `#` inside quoted context is not a concern —
// dists.dss does not use quoting.
func stripHashComment(line string) string {
	before, _, _ := strings.Cut(line, "#")

	return before
}
