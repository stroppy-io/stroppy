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

// parseStream reads a whole dists.dss source from r and returns the
// distributions in declaration order.
func parseStream(r io.Reader) (map[string]*dict, []string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	out := map[string]*dict{}
	var order []string
	var cur *block

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		line := strings.TrimSpace(stripHashComment(raw))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)

		switch {
		case strings.HasPrefix(lower, "begin "):
			if cur != nil {
				return nil, nil, fmt.Errorf(
					"tpch-dists: line %d: BEGIN %q while %q still open",
					lineNum, line[len("BEGIN "):], cur.name,
				)
			}
			name := strings.TrimSpace(line[len("begin "):])
			if name == "" {
				return nil, nil, fmt.Errorf("tpch-dists: line %d: BEGIN missing name", lineNum)
			}
			cur = &block{name: name}

		case strings.HasPrefix(lower, "end "):
			if cur == nil {
				return nil, nil, fmt.Errorf("tpch-dists: line %d: END with no matching BEGIN", lineNum)
			}
			name := strings.TrimSpace(line[len("end "):])
			if !strings.EqualFold(name, cur.name) {
				return nil, nil, fmt.Errorf(
					"tpch-dists: line %d: END %q does not match BEGIN %q",
					lineNum, name, cur.name,
				)
			}
			if cur.declared > 0 && cur.declared != len(cur.rows) {
				return nil, nil, fmt.Errorf(
					"tpch-dists: line %d: block %q declared COUNT=%d but has %d rows",
					lineNum, cur.name, cur.declared, len(cur.rows),
				)
			}
			if _, dup := out[cur.name]; dup {
				return nil, nil, fmt.Errorf("tpch-dists: line %d: duplicate dist %q", lineNum, cur.name)
			}
			out[cur.name] = blockToDict(cur)
			order = append(order, cur.name)
			cur = nil

		default:
			if cur == nil {
				return nil, nil, fmt.Errorf(
					"tpch-dists: line %d: data line outside BEGIN/END: %q",
					lineNum, line,
				)
			}
			if err := parseDataLine(line, cur); err != nil {
				return nil, nil, fmt.Errorf("tpch-dists: line %d: %w", lineNum, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("tpch-dists: scan: %w", err)
	}
	if cur != nil {
		return nil, nil, fmt.Errorf("tpch-dists: unterminated block %q", cur.name)
	}
	return out, order, nil
}

// parseDataLine handles either `COUNT|N` or `<value>|<weight>`.
func parseDataLine(line string, cur *block) error {
	parts := strings.SplitN(line, "|", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected `a|b`, got %q", line)
	}
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	if strings.EqualFold(left, "count") {
		n, err := strconv.Atoi(right)
		if err != nil {
			return fmt.Errorf("COUNT value: %w", err)
		}
		if cur.declared > 0 {
			return errors.New("duplicate COUNT in block")
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

// blockToDict materialises the uniform Dict-shaped JSON.
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
