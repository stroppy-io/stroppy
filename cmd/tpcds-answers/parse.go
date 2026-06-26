package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// block is one result set: column names plus rows of stringified cells.
type block struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// errParse is the sentinel wrapped by structural parse errors.
var errParse = errors.New("parse error")

// rowsFooter matches the result-count trailers both source DBs append: psql's
// "(42 rows)" and the kit's sqlplus-style "100 rows selected." /
// "19 record(s) selected." — none of which are data.
var rowsFooter = regexp.MustCompile(`^(\(\s*\d+\s+rows?\s*\)|\d+\s+(rows?|records?|record\(s\))\s+selected\.?)\s*$`)

// outputMarker matches the "---------------------- OUTPUT Query 2" lines that
// separate the two result sets of a two-part query (q24). Treated as noise:
// blocks are delimited structurally by a header followed by a dashes line.
var outputMarker = regexp.MustCompile(`OUTPUT\s+Query`)

const maxScannerBuf = 1 << 20

// tabStop is the column width psql's output was unexpand-ed against. The
// kit's *.ans files replace runs of spaces with tabs at 8-column stops;
// expanding them back to spaces restores the alignment the dashes line
// encodes, so byte-offset slicing is exact.
const tabStop = 8

// minRulerDashes guards against a stray "---" in data being read as a ruler.
const minRulerDashes = 3

// headerAndRuler is the header + dashes line pair a fixed-width block opens with;
// data rows start that many lines below the header.
const headerAndRuler = 2

// parseAnswerBlocks parses one .ans file into its ordered result blocks.
// It auto-detects the two upstream formats:
//   - pipe-delimited (e.g. q39): a `col|col|col` header with no dashes line.
//   - psql fixed-width (the rest): a dashes line whose `-` runs give the
//     column spans; one or more header/dashes/rows blocks back to back.
func parseAnswerBlocks(r io.Reader) ([]block, error) {
	lines, err := collectLines(r)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: empty answer file", errParse)
	}

	for _, ln := range lines {
		if isDashesLine(ln) {
			return parseFixedWidth(lines)
		}
	}

	return parsePipe(lines)
}

// collectLines reads every line, expands tabs to restore alignment, trims
// trailing whitespace, and drops blank lines, OUTPUT markers, and row
// footers. Dashes lines are kept — they carry the column spans.
func collectLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, maxScannerBuf), maxScannerBuf)

	var lines []string

	for scanner.Scan() {
		ln := expandTabs(scanner.Text())

		ln = strings.TrimRight(ln, " \t\r")
		if ln == "" || rowsFooter.MatchString(ln) || outputMarker.MatchString(ln) {
			continue
		}

		lines = append(lines, ln)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	return lines, nil
}

// expandTabs replaces tabs with spaces, advancing to the next tabStop
// boundary — the inverse of the unexpand the kit applied.
func expandTabs(s string) string {
	var sb strings.Builder

	col := 0

	for _, ch := range s {
		if ch == '\t' {
			n := tabStop - (col % tabStop)
			for range n {
				sb.WriteByte(' ')
			}

			col += n

			continue
		}

		sb.WriteRune(ch)

		col++
	}

	return sb.String()
}

// isDashesLine reports whether a line is a column-span ruler: only '-' and
// ' ', with at least three dashes (so a stray "---" in data is not mistaken
// for a ruler, and pipe-format headers never match).
func isDashesLine(s string) bool {
	dashes := 0

	for _, r := range s {
		switch r {
		case '-':
			dashes++
		case ' ':
		default:
			return false
		}
	}

	return dashes >= minRulerDashes
}

// spansOf returns the [start,end) byte ranges of each run of '-'.
func spansOf(dashes string) [][2]int {
	var spans [][2]int

	start := -1

	for i, r := range dashes {
		if r == '-' {
			if start < 0 {
				start = i
			}

			continue
		}

		if start >= 0 {
			spans = append(spans, [2]int{start, i})
			start = -1
		}
	}

	if start >= 0 {
		spans = append(spans, [2]int{start, len(dashes)})
	}

	return spans
}

// sliceBySpans cuts a fixed-width line into trimmed cells at the given spans.
// A line shorter than a span yields an empty cell for that column.
func sliceBySpans(line string, spans [][2]int) []string {
	out := make([]string, len(spans))
	for i, sp := range spans {
		start, end := sp[0], sp[1]
		if start > len(line) {
			out[i] = ""

			continue
		}

		if end > len(line) {
			end = len(line)
		}

		out[i] = strings.TrimSpace(line[start:end])
	}

	return out
}

// headerNames derives column names for a block. The header line uses the same
// spans, but its (often tab-padded) text can be unreliable, so we fall back to
// whitespace-splitting and finally to generic names — names are not compared.
func headerNames(header string, spans [][2]int) []string {
	names := sliceBySpans(header, spans)
	for _, n := range names {
		if n == "" {
			return genericNames(len(spans))
		}
	}

	return names
}

func genericNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("col%d", i)
	}

	return out
}

// parseFixedWidth splits the filtered lines into blocks. A block is a header
// line immediately followed by a dashes line, then data rows up to the next
// header/dashes pair or EOF.
func parseFixedWidth(lines []string) ([]block, error) {
	var blocks []block

	i := 0
	for i < len(lines) {
		// A block opens at a header line followed by a dashes ruler.
		if i+1 >= len(lines) || !isDashesLine(lines[i+1]) {
			i++

			continue
		}

		spans := spansOf(lines[i+1])
		cols := headerNames(lines[i], spans)

		var rows [][]string

		pos := i + headerAndRuler
		for pos < len(lines) {
			// Stop before the next block's header (header + dashes pair).
			if pos+1 < len(lines) && isDashesLine(lines[pos+1]) {
				break
			}

			if isDashesLine(lines[pos]) {
				break
			}

			rows = append(rows, sliceBySpans(lines[pos], spans))
			pos++
		}

		blocks = append(blocks, block{Columns: cols, Rows: rows})
		i = pos
	}

	if len(blocks) == 0 {
		return nil, fmt.Errorf("%w: no header/dashes block found", errParse)
	}

	return blocks, nil
}

// parsePipe parses a `|`-delimited file (single block): first line is the
// header, the rest are rows.
func parsePipe(lines []string) ([]block, error) {
	if !strings.Contains(lines[0], "|") {
		return nil, fmt.Errorf("%w: pipe format expected, header has no '|'", errParse)
	}

	cols := splitPipe(lines[0])

	rows := make([][]string, 0, len(lines)-1)
	for _, ln := range lines[1:] {
		rows = append(rows, splitPipe(ln))
	}

	return []block{{Columns: cols, Rows: rows}}, nil
}

func splitPipe(line string) []string {
	parts := strings.Split(line, "|")

	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}

	return out
}

// readBlocks is a small helper for tests / callers operating on bytes.
func readBlocks(raw []byte) ([]block, error) {
	return parseAnswerBlocks(bytes.NewReader(raw))
}
