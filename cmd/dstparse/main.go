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
// The `.dst` files are upstream TPC-DS artefacts; this tool is the
// one-way boundary that imports them at build time. The generated JSON
// is what stroppy ships under workloads/tpcds/.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
		os.Exit(2)
	}

	info, err := os.Stat(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dstparse: stat %s: %v\n", *in, err)
		os.Exit(1)
	}

	var files []string
	if info.IsDir() {
		entries, err := os.ReadDir(*in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dstparse: read dir %s: %v\n", *in, err)
			os.Exit(1)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.EqualFold(filepath.Ext(e.Name()), ".dst") {
				files = append(files, filepath.Join(*in, e.Name()))
			}
		}
		sort.Strings(files)
		if len(files) == 0 {
			fmt.Fprintf(os.Stderr, "dstparse: no .dst files in %s\n", *in)
			os.Exit(1)
		}
	} else {
		files = []string{*in}
	}

	root := &doc{
		Version:       *version,
		Distributions: map[string]*dict{},
	}

	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dstparse: read %s: %v\n", path, err)
			os.Exit(1)
		}
		dists, err := parseStream(bytes.NewReader(raw))
		if err != nil {
			fmt.Fprintf(os.Stderr, "dstparse: parse %s: %v\n", path, err)
			os.Exit(1)
		}
		for _, nd := range dists {
			if _, dup := root.Distributions[nd.name]; dup {
				fmt.Fprintf(os.Stderr, "dstparse: duplicate distribution %q (in %s)\n", nd.name, path)
				os.Exit(1)
			}
			root.Distributions[nd.name] = nd.toDict()
		}
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
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "dstparse: write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "dstparse: wrote %s (%d distributions, %d bytes)\n",
		*out, len(root.Distributions), len(data))
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
