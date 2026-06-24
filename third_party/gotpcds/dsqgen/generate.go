package dsqgen

import "fmt"

// Query is one generated statement (or two, for the a/b queries 14/23/24/39,
// separated by ';').
type Query struct {
	Name string // template name, e.g. "query1"
	SQL  string
}

// Result is the output of one Generate call.
type Result struct {
	Queries []Query
	Skipped []string // template names that could not be fully generated, with reason
}

// Generate renders every template for one dialect/scale/seed/stream. Each query
// is seeded by (seed + query index) so a stream varies per query and is
// reproducible. Templates that reference an unsupported pseudo-distribution
// (e.g. query88's syllable-generated store names) are reported in Skipped rather
// than emitted with a broken parameter.
func Generate(dialect Dialect, scale float64, seed int64, streamNum int) (*Result, error) {
	tmpls, err := LoadTemplates()
	if err != nil {
		return nil, err
	}
	dc := newDistCache()
	res := &Result{}
	for i, t := range tmpls {
		if dialect.Name == "mysql" && mysqlStructuralGap(t.Name) {
			res.Skipped = append(res.Skipped, t.Name+": full outer join (use baked mysql.sql)")
			continue
		}
		ev := newEvaluator(seed+int64(i)*2654435761, scale, dc)
		env, err := ev.evalTemplate(t)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %v", t.Name, err))
			continue
		}
		sql := dialect.render(t, env, i+1, streamNum, seed)
		res.Queries = append(res.Queries, Query{Name: t.Name, SQL: sql})
	}
	// Each stream runs the queries in its own permutation (TPC-DS Clause 7.1.4).
	permuteQueries(res.Queries, seed^(int64(streamNum+1)*2654435761))
	return res, nil
}

// permuteQueries shuffles queries in place with a seeded Fisher-Yates, so a
// given (seed, stream) yields a stable, reproducible order.
func permuteQueries(qs []Query, seed int64) {
	r := newRNG(seed)
	for i := len(qs) - 1; i > 0; i-- {
		j := int(r.intn(0, int64(i)))
		qs[i], qs[j] = qs[j], qs[i]
	}
}
