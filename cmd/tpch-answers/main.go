// tpch-answers parses a directory of TPC-H reference answer files into
// a single `answers_sf1.json` document. Both `q*.out` and `*.ans`
// naming conventions are accepted; the key in the output map is the
// filename stem lower-cased (`q1.out` → `q1`).
//
// Usage:
//
//	tpch-answers -in <dir> -out <json-path> [-pretty] [-version <tag>]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// nameLike matches `q1`, `q1.out`, `q1.ans`, `q01.out`, `q1-sf1.out`,
// etc. — anything starting with `q<digits>`. Matched lower-cased.
var nameLike = regexp.MustCompile(`^q\d+`)

func main() {
	in := flag.String("in", "", "directory containing answer files (required)")
	out := flag.String("out", "", "output JSON path (stdout when omitted)")
	version := flag.String("version", "1", "schema version string embedded in output")
	sourceLabel := flag.String("source", "", "human-readable source label (defaults to input basename)")
	pretty := flag.Bool("pretty", true, "emit indented JSON (default true — answers files are human-reviewed)")
	flag.Parse()

	if *in == "" {
		fmt.Fprintln(os.Stderr, "tpch-answers: -in is required")
		flag.Usage()
		os.Exit(2)
	}

	info, err := os.Stat(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpch-answers: stat %s: %v\n", *in, err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "tpch-answers: -in must be a directory, got %s\n", *in)
		os.Exit(1)
	}

	entries, err := os.ReadDir(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpch-answers: read dir %s: %v\n", *in, err)
		os.Exit(1)
	}

	answers := map[string]*answer{}
	var seenStems []string

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".out" && ext != ".ans" {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		if !nameLike.MatchString(stem) {
			continue
		}
		// Normalise `q01` → `q1` so e.g. duckdb-style `q01.out` and
		// classic `q1.out` land on the same key. Strip leading zeros
		// from the numeric suffix after the `q`.
		stem = normaliseStem(stem)

		path := filepath.Join(*in, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tpch-answers: read %s: %v\n", path, err)
			os.Exit(1)
		}
		a, err := parseAnswerFile(bytes.NewReader(raw))
		if err != nil {
			fmt.Fprintf(os.Stderr, "tpch-answers: parse %s: %v\n", path, err)
			os.Exit(1)
		}
		if _, dup := answers[stem]; dup {
			fmt.Fprintf(os.Stderr, "tpch-answers: duplicate query key %q (from %s)\n", stem, name)
			os.Exit(1)
		}
		answers[stem] = a
		seenStems = append(seenStems, stem)
	}

	if len(answers) == 0 {
		fmt.Fprintf(os.Stderr, "tpch-answers: no q*.out / q*.ans files in %s\n", *in)
		os.Exit(1)
	}

	sort.Strings(seenStems)

	source := *sourceLabel
	if source == "" {
		source = filepath.Base(*in)
	}

	root := &doc{
		Version: *version,
		Source:  source,
		Answers: answers,
	}

	var data []byte
	if *pretty {
		data, err = json.MarshalIndent(root, "", "  ")
	} else {
		data, err = json.Marshal(root)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpch-answers: marshal: %v\n", err)
		os.Exit(1)
	}

	if *out == "" {
		_, _ = os.Stdout.Write(data)
		_, _ = os.Stdout.Write([]byte{'\n'})
		return
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "tpch-answers: write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "tpch-answers: wrote %s (%d queries, %d bytes)\n",
		*out, len(root.Answers), len(data))
}

// normaliseStem rewrites `q07` → `q7` while leaving `q10` alone.
// Everything after the numeric run is preserved (e.g. `q1-sf1`).
func normaliseStem(stem string) string {
	if !strings.HasPrefix(stem, "q") {
		return stem
	}
	end := 1
	for end < len(stem) && stem[end] >= '0' && stem[end] <= '9' {
		end++
	}
	if end == 1 {
		return stem
	}
	digits := strings.TrimLeft(stem[1:end], "0")
	if digits == "" {
		digits = "0"
	}
	return "q" + digits + stem[end:]
}
