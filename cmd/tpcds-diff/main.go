// tpcds-diff compares two TPC-DS query-result dumps produced by the workload's
// ANSWER_DUMP mode (lines "__TPCDS_DUMP__\t<name>\t<json-rows|ERR:msg>" in a run
// log). It is the cross-DB / pg-oracle check: run the same queries on two
// engines (or two scales), dump each, and diff. Rows compare as a sorted
// multiset (engines order ties / NULLs differently) with a numeric tolerance
// for decimal/float formatting; everything else compares exact.
//
// Usage:
//
//	tpcds-diff -ref pg.log -test mysql.log [-v]
//
// Exit code is non-zero if any query DIFFs, so it can gate CI.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	tolRel = 1e-3  // 0.1% relative
	tolAbs = 0.01  // or sub-cent absolute
	maxBuf = 1 << 26
)

var numericRe = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

// dump maps query name -> rows (each row a slice of normalized string cells),
// plus any per-query error string emitted by the workload.
type dump struct {
	rows map[string][][]string
	errs map[string]string
}

func main() {
	ref := flag.String("ref", "", "reference dump/log (the oracle, e.g. postgres) (required)")
	test := flag.String("test", "", "dump/log to check against ref (required)")
	verbose := flag.Bool("v", false, "print first mismatching cells per DIFF query")
	flag.Parse()
	if *ref == "" || *test == "" {
		fmt.Fprintln(os.Stderr, "tpcds-diff: -ref and -test are required")
		os.Exit(2)
	}

	a, err := load(*ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpcds-diff: ref: %v\n", err)
		os.Exit(2)
	}
	b, err := load(*test)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpcds-diff: test: %v\n", err)
		os.Exit(2)
	}

	names := unionKeys(a, b)
	var ok, diff, skip int
	for _, name := range names {
		line, status := compareOne(name, a, b, *verbose)
		switch status {
		case "ok":
			ok++
		case "diff":
			diff++
		default:
			skip++
		}
		if status != "ok" || *verbose {
			fmt.Println(line)
		}
	}
	fmt.Printf("===== tpcds-diff: ref=%s test=%s =====\n", *ref, *test)
	fmt.Printf("total=%d  ok=%d  diff=%d  skipped=%d\n", len(names), ok, diff, skip)
	if diff > 0 {
		os.Exit(1)
	}
}

// compareOne returns a report line and a status (ok|diff|skip) for one query.
func compareOne(name string, a, b *dump, verbose bool) (string, string) {
	if e, isErr := a.errs[name]; isErr {
		return fmt.Sprintf("  %-14s SKIP   ref error: %s", name, e), "skip"
	}
	if e, isErr := b.errs[name]; isErr {
		return fmt.Sprintf("  %-14s DIFF   test error: %s", name, e), "diff"
	}
	ar, aok := a.rows[name]
	br, bok := b.rows[name]
	if !aok || !bok {
		return fmt.Sprintf("  %-14s SKIP   present ref=%v test=%v", name, aok, bok), "skip"
	}
	sortRows(ar)
	sortRows(br)
	deltas := compareRows(ar, br)
	if len(deltas) == 0 && len(ar) == len(br) {
		return fmt.Sprintf("  %-14s OK     rows=%d", name, len(ar)), "ok"
	}
	preview := ""
	if verbose && len(deltas) > 0 {
		preview = "  " + strings.Join(deltas[:min(3, len(deltas))], "; ")
	}
	return fmt.Sprintf("  %-14s DIFF   rows ref=%d test=%d%s", name, len(ar), len(br), preview), "diff"
}

// compareRows compares two pre-sorted row sets positionally with tolerance.
func compareRows(ar, br [][]string) []string {
	var deltas []string
	n := len(ar)
	if len(br) > n {
		n = len(br)
	}
	for i := 0; i < n && len(deltas) < 5; i++ {
		if i >= len(ar) {
			deltas = append(deltas, fmt.Sprintf("row %d: extra in test", i))
			continue
		}
		if i >= len(br) {
			deltas = append(deltas, fmt.Sprintf("row %d: missing in test", i))
			continue
		}
		ra, rb := ar[i], br[i]
		w := len(ra)
		if len(rb) > w {
			w = len(rb)
		}
		for c := 0; c < w; c++ {
			if !cellsMatch(get(ra, c), get(rb, c)) {
				deltas = append(deltas, fmt.Sprintf("row %d col %d: ref=%q test=%q", i, c, get(ra, c), get(rb, c)))
				break
			}
		}
	}
	return deltas
}

func get(r []string, i int) string {
	if i < len(r) {
		return r[i]
	}
	return ""
}

func cellsMatch(a, b string) bool {
	if a == b {
		return true
	}
	if !numericRe.MatchString(a) || !numericRe.MatchString(b) {
		return false
	}
	x, _ := strconv.ParseFloat(a, 64)
	y, _ := strconv.ParseFloat(b, 64)
	d := math.Abs(x - y)
	if d <= tolAbs {
		return true
	}
	return d/math.Max(math.Abs(y), 1) <= tolRel
}

// rowKey rounds numeric cells to 2dp so near-equal rows sort together.
func rowKey(cells []string) string {
	var b strings.Builder
	for _, c := range cells {
		if numericRe.MatchString(c) {
			f, _ := strconv.ParseFloat(c, 64)
			b.WriteString(strconv.FormatFloat(f, 'f', 2, 64))
		} else {
			b.WriteString(c)
		}
		b.WriteByte(0x01)
	}
	return b.String()
}

func sortRows(rows [][]string) {
	sort.Slice(rows, func(i, j int) bool { return rowKey(rows[i]) < rowKey(rows[j]) })
}

func unionKeys(a, b *dump) []string {
	seen := map[string]bool{}
	for k := range a.rows {
		seen[k] = true
	}
	for k := range a.errs {
		seen[k] = true
	}
	for k := range b.rows {
		seen[k] = true
	}
	for k := range b.errs {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return queryLess(out[i], out[j]) })
	return out
}

// queryLess orders query_2 before query_10 (numeric, then suffix).
func queryLess(a, b string) bool {
	na, sa := splitQ(a)
	nb, sb := splitQ(b)
	if na != nb {
		return na < nb
	}
	return sa < sb
}

var qNumRe = regexp.MustCompile(`query_(\d+)(.*)`)

func splitQ(s string) (int, string) {
	m := qNumRe.FindStringSubmatch(s)
	if m == nil {
		return 1 << 30, s
	}
	n, _ := strconv.Atoi(m[1])
	return n, m[2]
}

// load parses a run log (or bare dump file), extracting __TPCDS_DUMP__ lines.
func load(path string) (*dump, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	d := &dump{rows: map[string][][]string{}, errs: map[string]string{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), maxBuf)
	for sc.Scan() {
		line := unwrapLogMsg(sc.Text())
		idx := strings.Index(line, "__TPCDS_DUMP__\t")
		if idx < 0 {
			continue
		}
		parts := strings.SplitN(line[idx+len("__TPCDS_DUMP__\t"):], "\t", 2)
		if len(parts) != 2 {
			continue
		}
		name, payload := parts[0], parts[1]
		if strings.HasPrefix(payload, "ERR:") {
			d.errs[name] = strings.TrimPrefix(payload, "ERR:")
			continue
		}
		rows, err := parseRows(payload)
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", name, err)
		}
		d.rows[name] = rows
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(d.rows) == 0 && len(d.errs) == 0 {
		return nil, fmt.Errorf("no __TPCDS_DUMP__ lines in %s", path)
	}
	return d, nil
}

// logMsgRe extracts the quoted message field from a k6/logrus text log line:
//   time="..." level=info msg="__TPCDS_DUMP__\tquery_1\t[[\"..\"]]" source=console
var logMsgRe = regexp.MustCompile(`msg=("(?:[^"\\]|\\.)*")`)

// unwrapLogMsg returns the unescaped logrus msg field if the line is a logrus
// record, else the line unchanged (so bare dump files also parse). The dump's
// tabs and JSON quotes are Go-escaped inside msg="..."; strconv.Unquote restores
// the real tabs/quotes the rest of load() expects.
func unwrapLogMsg(line string) string {
	m := logMsgRe.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	s, err := strconv.Unquote(m[1])
	if err != nil {
		return line
	}
	return s
}

// parseRows unmarshals the dumped JSON array-of-arrays of normalized cells.
func parseRows(payload string) ([][]string, error) {
	var rows [][]string
	if err := json.Unmarshal([]byte(payload), &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
