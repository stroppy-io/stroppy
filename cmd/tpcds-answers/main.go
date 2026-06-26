// tpcds-answers parses the official TPC-DS kit reference answer sets
// (`answer_sets/*.ans`) into a single `answers_sf1.json` document keyed by the
// query-statement names used in the baked per-dialect SQL (`query_1` …
// `query_99`, with the two-part queries 14/23/24 split into `_a`/`_b`).
//
// The kit ships one .ans per query at the SF=1 qualification database, in two
// formats (pipe-delimited and psql fixed-width with tab-packed padding); see
// parse.go. Queries with NULLS_FIRST/NULLS_LAST variants differ only in row
// order (same multiset), so one variant is kept. Query 39's answer joins its
// two parts horizontally into one table and is skipped (it still executes in
// the workload; it is just not value-checked).
//
// Usage:
//
//	tpcds-answers -in <kit>/answer_sets -out workloads/tpcds/answers_sf1.json
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	exitUsage   = 2
	outFilePerm = 0o600
	numQueries  = 99
	twoPartCnt  = 2 // a two-part query's .ans holds two fixed-width blocks
)

var errInput = errors.New("input error")

// twoPart are the queries whose template holds two statements; the baked SQL
// splits them into `_a`/`_b`. Their .ans carries two fixed-width blocks.
var twoPart = map[int]bool{14: true, 23: true, 24: true}

// skipQuery 39 joins its two parts horizontally into a single pipe table, so
// it does not map cleanly onto the `_a`/`_b` split. Left unvalidated.
var skipQuery = map[int]bool{39: true}

type doc struct {
	Version string            `json:"version"`
	Source  string            `json:"source"`
	Answers map[string]*block `json:"answers"`
}

func main() {
	in := flag.String("in", "", "kit answer_sets directory (required)")
	out := flag.String("out", "", "output JSON path (stdout when omitted)")
	version := flag.String("version", "4.0.0", "schema/source version tag")
	pretty := flag.Bool("pretty", true, "emit indented JSON")

	flag.Parse()

	if *in == "" {
		fmt.Fprintln(os.Stderr, "tpcds-answers: -in is required")
		flag.Usage()
		os.Exit(exitUsage)
	}

	answers, err := collectAnswers(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpcds-answers: %v\n", err)
		os.Exit(1)
	}

	root := &doc{Version: *version, Source: "TPC-DS kit answer_sets (SF=1 qualification)", Answers: answers}

	var data []byte
	if *pretty {
		data, err = json.MarshalIndent(root, "", "  ")
	} else {
		data, err = json.Marshal(root)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "tpcds-answers: marshal: %v\n", err)
		os.Exit(1)
	}

	if *out == "" {
		_, _ = os.Stdout.Write(append(data, '\n'))

		return
	}

	if err := os.WriteFile(*out, data, outFilePerm); err != nil {
		fmt.Fprintf(os.Stderr, "tpcds-answers: write %s: %v\n", *out, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "tpcds-answers: wrote %s (%d statements, %d bytes)\n", *out, len(root.Answers), len(data))
}

// collectAnswers parses one file per query number into the keyed answer map.
func collectAnswers(in string) (map[string]*block, error) {
	info, err := os.Stat(in)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", in, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%w: -in must be a directory, got %s", errInput, in)
	}

	answers := map[string]*block{}

	for qNum := 1; qNum <= numQueries; qNum++ {
		if skipQuery[qNum] {
			fmt.Fprintf(os.Stderr, "tpcds-answers: query %d skipped (horizontally-joined two-part answer)\n", qNum)

			continue
		}

		path, ok := answerPath(in, qNum)
		if !ok {
			fmt.Fprintf(os.Stderr, "tpcds-answers: query %d has no .ans file\n", qNum)

			continue
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}

		blocks, err := readBlocks(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
		}

		mapBlocks(answers, qNum, blocks, filepath.Base(path))
	}

	if len(answers) == 0 {
		return nil, fmt.Errorf("%w: no answers parsed from %s", errInput, in)
	}

	return answers, nil
}

// mapBlocks assigns parsed blocks to the statement key(s) for query qNum.
func mapBlocks(answers map[string]*block, qNum int, blocks []block, fname string) {
	if twoPart[qNum] {
		if len(blocks) < twoPartCnt {
			fmt.Fprintf(os.Stderr, "tpcds-answers: %s: query %d two-part, want 2 blocks got %d\n", fname, qNum, len(blocks))
		}

		for idx, suffix := range []string{"a", "b"} {
			if idx < len(blocks) {
				b := blocks[idx]
				answers[fmt.Sprintf("query_%d_%s", qNum, suffix)] = &b
			}
		}

		return
	}
	// A single-statement query that prints more than one block is a paginated
	// result (psql reprinted the header/dashes mid-output); merge the rows.
	merged := blocks[0]
	for _, b := range blocks[1:] {
		merged.Rows = append(merged.Rows, b.Rows...)
	}

	answers[fmt.Sprintf("query_%d", qNum)] = &merged
}

// answerPath finds the .ans file for query n, preferring the plain file, then
// the NULLS_LAST variant, then NULLS_FIRST (the variants are multiset-equal).
func answerPath(in string, n int) (string, bool) {
	for _, name := range []string{
		fmt.Sprintf("%d.ans", n),
		fmt.Sprintf("%d_NULLS_LAST.ans", n),
		fmt.Sprintf("%d_NULLS_FIRST.ans", n),
	} {
		p := filepath.Join(in, name)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}

	return "", false
}

// sortedKeys is used by tests to enumerate output deterministically.
func sortedKeys(m map[string]*block) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
