package dsdgen

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"strconv"
	"strings"
)

// distFS embeds the TPC-DS distribution tables, vendored verbatim from the
// Apache-2.0 Trino port (text form, not the binary tpcds.idx). Each file is a
// list of "value(s): weight(s)" lines with "--" comments.
//
//go:embed distributions/*.dst
var distFS embed.FS

// StringValuesDistribution is a loaded .dst file: one or more parallel value
// columns and one or more cumulative-weight columns. Mirrors
// StringValuesDistribution.java.
type StringValuesDistribution struct {
	values  [][]string
	weights [][]int // cumulative
}

// loadStringValuesDistribution parses filename into numValueFields value columns
// and numWeightFields cumulative-weight columns.
func loadStringValuesDistribution(filename string, numValueFields, numWeightFields int) (*StringValuesDistribution, error) {
	data, err := distFS.ReadFile("distributions/" + filename)
	if err != nil {
		return nil, fmt.Errorf("dsdgen: read distribution %q: %w", filename, err)
	}

	d := &StringValuesDistribution{
		values:  make([][]string, numValueFields),
		weights: make([][]int, numWeightFields),
	}
	prev := make([]int, numWeightFields)

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		parts := splitUnescaped(line, ':')
		if len(parts) != 2 {
			return nil, fmt.Errorf("dsdgen: %q: expected value:weight, got %q", filename, line)
		}

		vals := splitUnescaped(parts[0], ',')
		if len(vals) != numValueFields {
			return nil, fmt.Errorf("dsdgen: %q: expected %d values, got %d in %q", filename, numValueFields, len(vals), line)
		}
		for i, v := range vals {
			d.values[i] = append(d.values[i], v)
		}

		ws := splitUnescaped(parts[1], ',')
		if len(ws) != numWeightFields {
			return nil, fmt.Errorf("dsdgen: %q: expected %d weights, got %d in %q", filename, numWeightFields, len(ws), line)
		}
		for i, w := range ws {
			n, err := strconv.Atoi(strings.TrimSpace(w))
			if err != nil {
				return nil, fmt.Errorf("dsdgen: %q: bad weight %q: %w", filename, w, err)
			}
			prev[i] += n // weights are stored cumulatively
			d.weights[i] = append(d.weights[i], prev[i])
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("dsdgen: scan distribution %q: %w", filename, err)
	}

	return d, nil
}

// ValueAtIndex returns the raw value at valueIndex in value column valueListIndex.
func (d *StringValuesDistribution) ValueAtIndex(valueListIndex, valueIndex int) string {
	return d.values[valueListIndex][valueIndex]
}

// Size is the number of entries in the distribution.
func (d *StringValuesDistribution) Size() int { return len(d.values[0]) }

// splitUnescaped splits s on sep, ignoring separators escaped with a backslash,
// then trims each field and removes the escaping backslashes. Mirrors the
// negative-lookbehind splitting and "\\"-stripping in DistributionUtils.
func splitUnescaped(s string, sep byte) []string {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == sep && (i == 0 || s[i-1] != '\\') {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()

			continue
		}
		cur.WriteByte(s[i])
	}
	out = append(out, strings.TrimSpace(cur.String()))

	for i, v := range out {
		out[i] = strings.ReplaceAll(v, "\\", "")
	}

	return out
}
