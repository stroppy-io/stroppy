// Package main in cmd/tpch-answers parses the upstream TPC-H reference
// answer files (`q1.out`, `q2.out`, ... or `*.ans`) into a single JSON
// document keyed by query name.
//
// Each upstream answer file is pipe-separated:
//
//	col1|col2|col3            -- header
//	v1|v2|v3                  -- data
//	v1|v2|v3
//
// Some distributions ship files with a few lines of preamble (run
// timestamp, query id, "X rows affected") before the header. The
// parser tolerates this by scanning forward until it finds the first
// non-empty line whose `|` count matches every following non-empty
// line's `|` count — that line is treated as the header. Trailing
// blank lines (and "(N rows)" footers) are ignored.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// answer is the JSON shape emitted per query.
type answer struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// doc is the top-level JSON document emitted by tpch-answers.
type doc struct {
	Version string             `json:"version"`
	Source  string             `json:"source"`
	Answers map[string]*answer `json:"answers"`
}

// rowsFooter matches lines like `(42 rows)` — emitted by some PSQL
// dumps — so we skip them at the tail of the file.
var rowsFooter = regexp.MustCompile(`^\(\s*\d+\s+rows?\s*\)\s*$`)

// parseAnswerFile reads one answer file and returns its parsed form.
func parseAnswerFile(r io.Reader) (*answer, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	// Collect every non-skipped line with line number for error
	// reporting; decide header boundary afterwards.
	type lineRec struct {
		num  int
		text string
	}

	var lines []lineRec
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		t := strings.TrimRight(scanner.Text(), " \t\r")
		if t == "" {
			continue
		}
		if rowsFooter.MatchString(t) {
			continue
		}
		// psql-style row separators like `-----+-----+-----` are noise.
		if isSeparatorLine(t) {
			continue
		}
		lines = append(lines, lineRec{num: lineNum, text: t})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	if len(lines) == 0 {
		return nil, errors.New("empty answer file")
	}

	// Pick the header: the first line that contains a `|`. Preamble
	// lines without `|` are skipped. After locking the header we require
	// every subsequent line to carry the same pipe count — mixed column
	// widths are a corrupt file, not a tolerable quirk.
	headerIdx := -1
	for i, ln := range lines {
		if strings.Contains(ln.text, "|") {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil, fmt.Errorf(
			"line %d: cannot identify header (no pipe-separated line found)",
			lines[0].num,
		)
	}
	wantPipes := strings.Count(lines[headerIdx].text, "|")
	for _, ln := range lines[headerIdx+1:] {
		got := strings.Count(ln.text, "|")
		if got != wantPipes {
			return nil, fmt.Errorf(
				"line %d: cannot identify header (got %d pipes, header declared %d)",
				ln.num, got, wantPipes,
			)
		}
	}

	header := splitPipe(lines[headerIdx].text)
	rows := make([][]string, 0, len(lines)-headerIdx-1)
	for _, ln := range lines[headerIdx+1:] {
		cells := splitPipe(ln.text)
		if len(cells) != len(header) {
			return nil, fmt.Errorf(
				"line %d: got %d columns, header declares %d",
				ln.num, len(cells), len(header),
			)
		}
		rows = append(rows, cells)
	}

	return &answer{Columns: header, Rows: rows}, nil
}

// splitPipe splits on `|` and trims whitespace from each field.
func splitPipe(line string) []string {
	parts := strings.Split(line, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

// isSeparatorLine reports whether a line is a psql-style row separator
// composed only of `-`, `+`, and whitespace.
func isSeparatorLine(s string) bool {
	seenDash := false
	for _, r := range s {
		switch r {
		case '-':
			seenDash = true
		case '+', ' ', '\t':
			// allowed
		default:
			return false
		}
	}
	return seenDash
}
