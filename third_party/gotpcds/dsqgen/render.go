package dsqgen

import (
	"regexp"
	"strconv"
	"strings"
)

// Dialect controls how a rendered query is adapted to a target SQL engine.
// The generator renders the ANSI template body, injects parameter values, then
// applies the dialect's syntactic transforms. Structural rewrites that are not
// expressible as text transforms (derived-table aliasing, full-outer-join
// emulation, query-6 decorrelation for MySQL) are NOT applied here — those live
// in the hand-maintained baked .sql files; see the package docs.
type Dialect struct {
	Name      string
	transform func(query, sql string) string
}

// Postgres and MySQL dialects. Postgres needs only the universal fixes plus
// integer-day date arithmetic; MySQL adds interval dates, WITH ROLLUP, CONCAT,
// function-paren spacing, and signed casts.
var (
	Postgres = Dialect{Name: "postgres", transform: pgTransform}
	MySQL    = Dialect{Name: "mysql", transform: mysqlTransform}
)

func DialectByName(name string) (Dialect, bool) {
	switch strings.ToLower(name) {
	case "postgres", "pg":
		return Postgres, true
	case "mysql":
		return MySQL, true
	default:
		return Dialect{}, false
	}
}

var refRe = regexp.MustCompile(`\[\s*([A-Za-z_][A-Za-z0-9_]*)(?:\.([A-Za-z0-9]+))?\s*\]`)

// render substitutes the body's [PLACEHOLDER]s with evaluated values and dialect
// limit macros, then applies the dialect transform.
func (d Dialect) render(t *Template, env map[string][]cell, queryNum, streamNum int, seed int64) string {
	limit := "100"
	if v, ok := env["_LIMIT"]; ok && len(v) > 0 {
		limit = v[0].str()
	}

	body := refRe.ReplaceAllStringFunc(t.Body, func(m string) string {
		sub := refRe.FindStringSubmatch(m)
		name, suf := sub[1], sub[2]
		switch name {
		case "_LIMITA", "_LIMITB", "_BEGIN", "_END":
			return ""
		case "_LIMITC":
			return "limit " + limit
		case "_LIMIT":
			return limit
		case "_QUERY":
			return strconv.Itoa(queryNum)
		case "_STREAM":
			return strconv.Itoa(streamNum)
		case "_TEMPLATE":
			return t.Name + ".tpl"
		case "_SEED":
			return strconv.FormatInt(seed, 10)
		}
		vals, ok := env[name]
		if !ok {
			return m // unresolved (e.g. stores fallback) — leave the marker visible
		}
		idx := 0
		if n, err := strconv.Atoi(suf); err == nil && n > 0 {
			idx = n - 1
		}
		if idx >= len(vals) {
			return m
		}
		return vals[idx].str()
	})

	body = strings.TrimSpace(body)
	if d.transform != nil {
		body = d.transform(t.Name, body)
	}
	return body
}

// --- transforms (mirror the hand-applied rules used to build the baked .sql) ---

var (
	daysParenRe  = regexp.MustCompile(`([+-]\s*\d+)\s+days\)`)
	daysIntervRe = regexp.MustCompile(`([+-])\s*(\d+)\s+days\)`)
	rollupRe     = regexp.MustCompile(`(?i)(group by)\s+rollup\s*\(([^)]*)\)`)
	lochierRe    = regexp.MustCompile(`(?i)(grouping\([^)]*\)\s*\+\s*grouping\([^)]*\))\s+as\s+lochierarchy`)
	lochUseRe    = regexp.MustCompile(`(?i)when\s+lochierarchy\s*=\s*0`)
	funcSpaceRe  = regexp.MustCompile(`(?i)\b(sum|avg|count|min|max|cast|coalesce|stddev_samp|grouping|substr|substring|nullif|lower|upper|round|abs|rank|dense_rank)\s+\(`)
	asIntRe      = regexp.MustCompile(`(?i)\bas\s+int\s*\)`)
)

// universal fixes applied to every dialect.
func universalTransform(sql string) string {
	sql = strings.ReplaceAll(sql, "c_last_review_date_sk", "c_last_review_date")
	// lochierarchy alias cannot be referenced inside an ORDER BY expression.
	if m := lochierRe.FindStringSubmatch(sql); m != nil {
		sql = lochUseRe.ReplaceAllString(sql, "when "+m[1]+" = 0")
	}
	// guard query 90's ratio against an empty denominator.
	sql = strings.ReplaceAll(sql,
		"cast(amc as decimal(15,4))/cast(pmc as decimal(15,4))",
		"cast(amc as decimal(15,4))/nullif(cast(pmc as decimal(15,4)),0)")
	return sql
}

func pgTransform(_, sql string) string {
	sql = universalTransform(sql)
	// Postgres: date + integer adds days.
	sql = daysParenRe.ReplaceAllString(sql, "$1)")
	return sql
}

func mysqlTransform(query, sql string) string {
	sql = universalTransform(sql)
	// MySQL: interval date arithmetic.
	sql = daysIntervRe.ReplaceAllString(sql, "$1 interval $2 day)")
	// GROUP BY ... WITH ROLLUP.
	sql = rollupRe.ReplaceAllString(sql, "$1 $2 with rollup")
	// '||' is logical OR in MySQL; the templates use it for string concat.
	sql = rewritePipeConcat(sql)
	// No space between a built-in function name and '('.
	sql = funcSpaceRe.ReplaceAllString(sql, "$1(")
	// CAST target int -> signed.
	sql = asIntRe.ReplaceAllString(sql, "as signed)")
	// Every FROM/JOIN derived table needs an alias.
	sql = aliasDerivedTables(sql)
	// Decorrelate query 6's per-category average (O(n²) on MySQL otherwise).
	if query == "query6" {
		sql = decorrelateQ6(sql)
	}
	return sql
}

// mysqlStructuralGap reports query names whose MySQL form needs a structural
// rewrite the stream generator does not apply (full outer join). They are
// correct in the baked workloads/tpcds/mysql.sql.
func mysqlStructuralGap(query string) bool { return fullOuterJoinQueries[query] }

var (
	pipeWordRe = regexp.MustCompile(`'([A-Za-z_]+)'\s*\|\|\s*([A-Za-z_][A-Za-z0-9_]*)`)
)

func rewritePipeConcat(sql string) string {
	sql = pipeWordRe.ReplaceAllString(sql, "concat('$1', $2)")
	sql = strings.ReplaceAll(sql,
		"'ORIENTAL' || ',' || 'BOXBUNDLES'",
		"concat('ORIENTAL', ',', 'BOXBUNDLES')")
	sql = strings.ReplaceAll(sql,
		"coalesce(c_last_name,'') || ', ' || coalesce(c_first_name,'')",
		"concat(coalesce(c_last_name,''), ', ', coalesce(c_first_name,''))")
	return sql
}
