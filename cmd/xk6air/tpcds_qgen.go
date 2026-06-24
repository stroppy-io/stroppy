package xk6air

import (
	"errors"
	"fmt"
	"strings"

	"github.com/stroppy-io/stroppy/third_party/gotpcds/dsqgen"
)

// errUnknownDialect is returned when an unsupported dialect name is requested.
var errUnknownDialect = errors.New("dsqgen: unknown dialect")

// GenerateTpcdsQueries renders one TPC-DS query stream in-process for the given
// dialect/scale/seed/stream and returns one {name, sql} object per statement
// (the a/b queries 14/23/24/39 are split). Exposed to workloads so stream
// generation happens inside the test run — no offline CLI/file step.
//
// Values are valid and scale-correct (drawn from the same distributions and
// rowcounts the data uses) but not byte-identical to the C dsqgen stream.
func GenerateTpcdsQueries(dialect string, scale float64, seed int64, stream int) ([]map[string]string, error) {
	d, ok := dsqgen.DialectByName(dialect)
	if !ok {
		return nil, fmt.Errorf("%w: %q", errUnknownDialect, dialect)
	}

	res, err := dsqgen.Generate(d, scale, seed, stream)
	if err != nil {
		return nil, err
	}

	suffix := []string{"_a", "_b", "_c"}

	out := make([]map[string]string, 0, len(res.Queries))
	for _, q := range res.Queries {
		var stmts []string

		for _, p := range strings.Split(q.SQL, ";") {
			if s := strings.TrimSpace(p); s != "" {
				stmts = append(stmts, s)
			}
		}

		for i, st := range stmts {
			name := q.Name
			if len(stmts) > 1 {
				name += suffix[i]
			}

			out = append(out, map[string]string{"name": name, "sql": st})
		}
	}

	return out, nil
}
