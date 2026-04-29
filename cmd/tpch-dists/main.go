// tpch-dists transforms upstream TPC-H `dists.dss` into the uniform
// Dict-shaped JSON document consumed by the relations data generator.
//
// Usage:
//
//	tpch-dists -in <dists.dss> -out <json-path> [-pretty] [-version <tag>]
//
// The generated JSON is what stroppy ships under workloads/tpch/.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// exitUsage is the process exit code used for CLI usage errors.
const exitUsage = 2

// outFilePerm is the permission mode for emitted JSON files. Parsed
// reference data is not secret but does not need to be world-readable.
const outFilePerm = 0o600

func main() {
	in := flag.String("in", "", "path to dists.dss (required)")
	out := flag.String("out", "", "output JSON path (stdout when omitted)")
	version := flag.String("version", "1", "schema version string embedded in output")
	sourceLabel := flag.String("source", "", "human-readable source label (defaults to input basename)")
	pretty := flag.Bool("pretty", false, "emit indented JSON")

	flag.Parse()

	if *in == "" {
		fmt.Fprintln(os.Stderr, "tpch-dists: -in is required")
		flag.Usage()
		os.Exit(exitUsage)
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpch-dists: read %s: %v\n", *in, err)
		os.Exit(1)
	}

	dists, _, err := parseStream(bytes.NewReader(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "tpch-dists: %v\n", err)
		os.Exit(1)
	}

	source := *sourceLabel
	if source == "" {
		source = filepath.Base(*in)
	}

	root := &doc{
		Version:       *version,
		Source:        source,
		Distributions: dists,
	}

	var data []byte
	if *pretty {
		data, err = json.MarshalIndent(root, "", "  ")
	} else {
		data, err = json.Marshal(root)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "tpch-dists: marshal: %v\n", err)
		os.Exit(1)
	}

	if *out == "" {
		_, _ = os.Stdout.Write(data)
		_, _ = os.Stdout.Write([]byte{'\n'})

		return
	}

	if err := os.WriteFile(*out, data, outFilePerm); err != nil {
		fmt.Fprintf(os.Stderr, "tpch-dists: write %s: %v\n", *out, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "tpch-dists: wrote %s (%d distributions, %d bytes)\n",
		*out, len(root.Distributions), len(data))
}
