package dsqgen

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/stroppy-io/stroppy/third_party/gotpcds/dsdgen"
)

// distFile maps a template distribution name to the vendored .dst filename.
// Names differ between the query templates and the data-generator's tables.
// "stores" and the "active_*" rowcount subsets are intentionally absent: they
// are scale-derived pseudo-distributions, handled by the evaluator's fallback.
var distFile = map[string]string{
	"fips_county":       "fips.dst",
	"categories":        "categories.dst",
	"cities":            "cities.dst",
	"marital_status":    "marital_statuses.dst",
	"education":         "education.dst",
	"gender":            "genders.dst",
	"colors":            "colors.dst",
	"i_manager_id":      "item_manager_id.dst",
	"units":             "units.dst",
	"sizes":             "sizes.dst",
	"return_reasons":    "return_reasons.dst",
	"ship_mode_carrier": "ship_mode_carrier.dst",
}

// distribution is a loaded .dst: parallel trimmed value columns. Weights are
// ignored — the generator picks members uniformly, which keeps values valid
// (every member exists in the data) without needing the weight-column names.
type distribution struct {
	values [][]string // [valueColumn][row]
}

func (d *distribution) size() int { return len(d.values[0]) }

// at returns the trimmed value at 1-based column col, row index idx (0-based,
// wrapped into range).
func (d *distribution) at(col1based, idx int) (string, error) {
	c := col1based - 1
	if c < 0 || c >= len(d.values) {
		return "", fmt.Errorf("distribution column %d out of range (%d columns)", col1based, len(d.values))
	}
	col := d.values[c]
	if len(col) == 0 {
		return "", fmt.Errorf("empty distribution column %d", col1based)
	}
	return col[((idx%len(col))+len(col))%len(col)], nil
}

// distCache loads and memoizes distributions by template name.
type distCache struct {
	m map[string]*distribution
}

func newDistCache() *distCache { return &distCache{m: map[string]*distribution{}} }

func (c *distCache) get(name string) (*distribution, error) {
	if d, ok := c.m[name]; ok {
		return d, nil
	}
	file, ok := distFile[name]
	if !ok {
		// Fall back to "<name>.dst" — many class distributions (women_class,
		// shoe_class, …) are vendored under their own name and reached via a
		// nested distmember that yields the dist name dynamically.
		file = name + ".dst"
	}
	d, err := loadDist(file)
	if err != nil {
		return nil, fmt.Errorf("unsupported distribution %q: %w", name, err)
	}
	c.m[name] = d
	return d, nil
}

// loadDist parses a .dst file, auto-detecting the value-column count from the
// first data line (value fields are the comma-separated tokens before ':').
func loadDist(file string) (*distribution, error) {
	raw, err := dsdgen.ReadDistFile(file)
	if err != nil {
		return nil, fmt.Errorf("dsqgen: %w", err)
	}

	var d *distribution
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		valPart := line
		if i := strings.IndexByte(line, ':'); i >= 0 {
			valPart = line[:i]
		}
		vals := strings.Split(valPart, ",")
		if d == nil {
			d = &distribution{values: make([][]string, len(vals))}
		}
		if len(vals) != len(d.values) {
			continue // skip malformed/continuation lines
		}
		for i, v := range vals {
			d.values[i] = append(d.values[i], strings.TrimSpace(v))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("dsqgen: scan %q: %w", file, err)
	}
	if d == nil || d.size() == 0 {
		return nil, fmt.Errorf("dsqgen: empty distribution %q", file)
	}
	return d, nil
}
