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

// maxScannerBuf bounds the bufio.Scanner buffer used when reading
// answer files line-by-line.
const maxScannerBuf = 1 << 20

// errParse is the sentinel wrapped by every structural parse error.
var errParse = errors.New("parse error")

// lineRec captures one non-skipped input line plus its 1-based source
// line number for error reporting.
type lineRec struct {
	num  int
	text string
}

// parseAnswerFile reads one answer file and returns its parsed form.
func parseAnswerFile(r io.Reader) (*answer, error) {
	lines, err := collectLines(r)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: empty answer file", errParse)
	}

	headerIdx, err := findHeader(lines)
	if err != nil {
		return nil, err
	}

	header := splitPipe(lines[headerIdx].text)

	rows, err := parseRows(lines[headerIdx+1:], header)
	if err != nil {
		return nil, err
	}

	return &answer{Columns: header, Rows: rows}, nil
}

// collectLines reads the answer file, dropping blank lines, PSQL row
// separators, and `(N rows)` footers. It returns every surviving line
// with its 1-based source line number.
func collectLines(r io.Reader) ([]lineRec, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, maxScannerBuf), maxScannerBuf)

	var lines []lineRec

	lineNum := 0
	for scanner.Scan() {
		lineNum++

		trimmed := strings.TrimRight(scanner.Text(), " \t\r")
		if trimmed == "" {
			continue
		}

		if rowsFooter.MatchString(trimmed) {
			continue
		}
		// psql-style row separators like `-----+-----+-----` are noise.
		if isSeparatorLine(trimmed) {
			continue
		}

		lines = append(lines, lineRec{num: lineNum, text: trimmed})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	return lines, nil
}

// findHeader picks the first line containing `|` as the header, then
// requires every subsequent line to carry the same pipe count. Mixed
// column widths are a corrupt file, not a tolerable quirk.
func findHeader(lines []lineRec) (int, error) {
	headerIdx := -1

	for i, ln := range lines {
		if strings.Contains(ln.text, "|") {
			headerIdx = i

			break
		}
	}

	if headerIdx < 0 {
		return 0, fmt.Errorf(
			"%w: line %d: cannot identify header (no pipe-separated line found)",
			errParse, lines[0].num,
		)
	}

	wantPipes := strings.Count(lines[headerIdx].text, "|")
	for _, ln := range lines[headerIdx+1:] {
		got := strings.Count(ln.text, "|")
		if got != wantPipes {
			return 0, fmt.Errorf(
				"%w: line %d: cannot identify header (got %d pipes, header declared %d)",
				errParse, ln.num, got, wantPipes,
			)
		}
	}

	return headerIdx, nil
}

// parseRows splits each data line into cells and checks against the
// header's column count.
func parseRows(data []lineRec, header []string) ([][]string, error) {
	rows := make([][]string, 0, len(data))
	for _, ln := range data {
		cells := splitPipe(ln.text)
		if len(cells) != len(header) {
			return nil, fmt.Errorf(
				"%w: line %d: got %d columns, header declares %d",
				errParse, ln.num, len(cells), len(header),
			)
		}

		rows = append(rows, cells)
	}

	return rows, nil
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
