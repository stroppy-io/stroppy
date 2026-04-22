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
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// exitUsage is the process exit code used for CLI usage errors.
const exitUsage = 2

// outFilePerm is the permission mode for emitted JSON files. Parsed
// reference data is not secret but does not need to be world-readable.
const outFilePerm = 0o600

// errInput is the sentinel wrapped by CLI input errors.
var errInput = errors.New("input error")

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
		os.Exit(exitUsage)
	}

	if err := validateInputDir(*in); err != nil {
		fmt.Fprintf(os.Stderr, "tpch-answers: %v\n", err)
		os.Exit(1)
	}

	answers, err := collectAnswers(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpch-answers: %v\n", err)
		os.Exit(1)
	}

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

	if err := os.WriteFile(*out, data, outFilePerm); err != nil {
		fmt.Fprintf(os.Stderr, "tpch-answers: write %s: %v\n", *out, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "tpch-answers: wrote %s (%d queries, %d bytes)\n",
		*out, len(root.Answers), len(data))
}

// validateInputDir confirms that in names an existing directory.
func validateInputDir(in string) error {
	info, err := os.Stat(in)
	if err != nil {
		return fmt.Errorf("stat %s: %w", in, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%w: -in must be a directory, got %s", errInput, in)
	}

	return nil
}

// collectAnswers walks the directory, filters for `q*.out` / `q*.ans`
// entries and parses each one. Returns a non-empty answers map or an
// error.
func collectAnswers(in string) (map[string]*answer, error) {
	entries, err := os.ReadDir(in)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", in, err)
	}

	answers := map[string]*answer{}

	var seenStems []string

	for _, entry := range entries {
		stem, path, ok := answerCandidate(entry, in)
		if !ok {
			continue
		}

		ans, err := parseAnswerPath(path)
		if err != nil {
			return nil, err
		}

		if _, dup := answers[stem]; dup {
			return nil, fmt.Errorf("%w: duplicate query key %q (from %s)", errInput, stem, entry.Name())
		}

		answers[stem] = ans
		seenStems = append(seenStems, stem)
	}

	if len(answers) == 0 {
		return nil, fmt.Errorf("%w: no q*.out / q*.ans files in %s", errInput, in)
	}

	sort.Strings(seenStems)

	return answers, nil
}

// answerCandidate reports whether the directory entry is an answer file
// we should parse, returning its normalised stem and absolute path.
func answerCandidate(entry os.DirEntry, in string) (stem, path string, ok bool) {
	if entry.IsDir() {
		return "", "", false
	}

	name := entry.Name()

	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".out" && ext != ".ans" {
		return "", "", false
	}

	stem = strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	if !nameLike.MatchString(stem) {
		return "", "", false
	}
	// Normalise `q01` → `q1` so e.g. duckdb-style `q01.out` and
	// classic `q1.out` land on the same key. Strip leading zeros
	// from the numeric suffix after the `q`.
	stem = normaliseStem(stem)

	return stem, filepath.Join(in, name), true
}

// parseAnswerPath reads and parses a single answer file.
func parseAnswerPath(path string) (*answer, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	ans, err := parseAnswerFile(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return ans, nil
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
