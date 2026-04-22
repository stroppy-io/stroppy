// dstparse transforms TPC-DS dsdgen `.dst` distribution files into the
// uniform Dict-shaped JSON document consumed by the relations data
// generator. Given a directory the tool parses every `.dst` file in it
// (non-recursive) and merges the distributions into one document; given
// a single file it parses just that one.
//
// Usage:
//
//	dstparse -in <dir-or-file> [-out <path>] [-pretty] [-version <tag>]
//
// The `.dst` files are upstream TPC-DS artifacts; this tool is the
// one-way boundary that imports them at build time. The generated JSON
// is what stroppy ships under workloads/tpcds/.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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

func main() {
	in := flag.String("in", "", "directory or single .dst file (required)")
	out := flag.String("out", "", "output JSON path (stdout when omitted)")
	version := flag.String("version", "1", "schema version string embedded in output")
	sourceLabel := flag.String("source", "", "human-readable source label (defaults to input basename)")
	pretty := flag.Bool("pretty", false, "emit indented JSON")

	flag.Parse()

	if *in == "" {
		fmt.Fprintln(os.Stderr, "dstparse: -in is required")
		flag.Usage()
		os.Exit(exitUsage)
	}

	info, err := os.Stat(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dstparse: stat %s: %v\n", *in, err)
		os.Exit(1)
	}

	files, err := collectInputFiles(*in, info.IsDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "dstparse: %v\n", err)
		os.Exit(1)
	}

	root := &doc{
		Version:       *version,
		Distributions: map[string]*dict{},
	}

	if err := mergeFiles(root, files); err != nil {
		fmt.Fprintf(os.Stderr, "dstparse: %v\n", err)
		os.Exit(1)
	}

	root.Source = buildSourceLabel(*sourceLabel, *in, files, info.IsDir())

	var data []byte
	if *pretty {
		data, err = json.MarshalIndent(root, "", "  ")
	} else {
		data, err = json.Marshal(root)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "dstparse: marshal: %v\n", err)
		os.Exit(1)
	}

	if *out == "" {
		_, _ = os.Stdout.Write(data)
		_, _ = os.Stdout.Write([]byte{'\n'})

		return
	}

	if err := os.WriteFile(*out, data, outFilePerm); err != nil {
		fmt.Fprintf(os.Stderr, "dstparse: write %s: %v\n", *out, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "dstparse: wrote %s (%d distributions, %d bytes)\n",
		*out, len(root.Distributions), len(data))
}

// mergeFiles parses each .dst file in turn and merges its distributions
// into root. Duplicate distribution names across files are rejected.
func mergeFiles(root *doc, files []string) error {
	for _, path := range files {
		if err := mergeOneFile(root, path); err != nil {
			return err
		}
	}

	return nil
}

// mergeOneFile reads, parses and merges a single .dst file into root.
func mergeOneFile(root *doc, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	dists, err := parseStream(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	for _, nd := range dists {
		if _, dup := root.Distributions[nd.name]; dup {
			return fmt.Errorf("%w: duplicate distribution %q (in %s)", errInput, nd.name, path)
		}

		root.Distributions[nd.name] = nd.toDict()
	}

	return nil
}

// collectInputFiles resolves the -in flag into a concrete list of .dst
// files to parse. For a single-file input the slice has one element; for
// a directory the slice contains every `*.dst` entry (non-recursive) in
// sorted order. Returns a usage-style error when the directory holds no
// .dst files.
func collectInputFiles(in string, isDir bool) ([]string, error) {
	if !isDir {
		return []string{in}, nil
	}

	entries, err := os.ReadDir(in)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", in, err)
	}

	var files []string

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		if strings.EqualFold(filepath.Ext(e.Name()), ".dst") {
			files = append(files, filepath.Join(in, e.Name()))
		}
	}

	sort.Strings(files)

	if len(files) == 0 {
		return nil, fmt.Errorf("%w: no .dst files in %s", errInput, in)
	}

	return files, nil
}

func buildSourceLabel(explicit, in string, files []string, isDir bool) string {
	if explicit != "" {
		return explicit
	}

	base := filepath.Base(in)
	if !isDir {
		return base
	}

	names := make([]string, len(files))
	for i, f := range files {
		names[i] = strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
	}

	return fmt.Sprintf("%s/{%s}", base, strings.Join(names, ","))
}
