package dsqgen

import (
	"fmt"
	"regexp"
	"strings"
)

// Structural MySQL rewrites that cannot be expressed as simple text
// substitutions. These mirror the per-query fixes baked into
// workloads/tpcds/mysql.sql.

var derivedKW = map[string]bool{
	"where": true, "group": true, "order": true, "having": true, "union": true,
	"on": true, "and": true, "or": true, "intersect": true, "except": true,
	"select": true, "from": true, "as": true, "by": true, "with": true, "limit": true,
}

// aliasDerivedTables gives every FROM/JOIN/comma derived table that lacks one a
// generated alias. MySQL and PostgreSQL before 16 require it. Scans with balanced
// parentheses, distinguishing FROM-position subqueries from scalar/IN subqueries
// by the preceding token.
func aliasDerivedTables(sql string) string {
	var b strings.Builder
	i, n, cnt := 0, len(sql), 0
	for i < n {
		c := sql[i]
		if c == '(' {
			j := i - 1
			for j >= 0 && isSpaceByte(sql[j]) {
				j--
			}
			k := j
			for k >= 0 && isIdentByte(sql[k]) {
				k--
			}
			prev := strings.ToLower(sql[k+1 : j+1])
			var prevch byte
			if j >= 0 {
				prevch = sql[j]
			}
			m := i + 1
			for m < n && isSpaceByte(sql[m]) {
				m++
			}
			startsSelect := m+6 <= n && strings.ToLower(sql[m:m+6]) == "select"
			fromPos := prev == "from" || prev == "join" || prevch == ','
			if startsSelect && fromPos {
				depth, p := 0, i
				for p < n {
					if sql[p] == '(' {
						depth++
					} else if sql[p] == ')' {
						depth--
						if depth == 0 {
							break
						}
					}
					p++
				}
				q := p + 1
				for q < n && isSpaceByte(sql[q]) {
					q++
				}
				r := q
				for r < n && isIdentByte(sql[r]) {
					r++
				}
				nxt := strings.ToLower(sql[q:r])
				hasAlias := r > q && !derivedKW[nxt]
				if !hasAlias && p < n {
					cnt++
					b.WriteString(sql[i : p+1])
					fmt.Fprintf(&b, " dt%d", cnt)
					i = p + 1
					continue
				}
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func isSpaceByte(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// q6 decorrelation: MySQL re-evaluates the correlated per-category average per
// row (O(n²)); replace it with a grouped-join derived table.
var (
	q6FromRe = regexp.MustCompile(`(,\s*item i\b)`)
	q6CorrRe = regexp.MustCompile(`(?is)i\.i_current_price\s*>\s*1\.2\s*\*\s*\(\s*select\s+avg\(j\.i_current_price\)\s*from\s+item j\s+where\s+j\.i_category\s*=\s*i\.i_category\s*\)`)
)

func decorrelateQ6(sql string) string {
	sql = q6FromRe.ReplaceAllString(sql,
		"$1\n     ,(select i_category, avg(i_current_price) avg_price from item group by i_category) icat")
	sql = q6CorrRe.ReplaceAllString(sql,
		"i.i_category = icat.i_category\n and i.i_current_price > 1.2 * icat.avg_price")
	return sql
}

var (
	q51FullJoinRe = regexp.MustCompile(`(?is)(select\s+case when web\.item_sk\b.*?from web_v1 web)\s+full outer join(\s+store_v1 store on\s*\([^)]*\))`)
	q97FullJoinRe = regexp.MustCompile(`(?is)select\s+sum\(case when ssci\.customer_sk\b.*?from ssci full outer join csci on\s*\([^)]*\)`)
)

// Keep the union below the window functions and aggregates. UNION ALL preserves
// duplicates; the anti-match filter prevents counting matched rows twice.
func emulateFullOuterJoin(query, sql string) string {
	switch query {
	case "query51":
		return q51FullJoinRe.ReplaceAllString(sql,
			"${1} left join${2}\n union all\n ${1} right join${2}\n where web.item_sk is null")
	case "query97":
		return q97FullJoinRe.ReplaceAllString(sql, `select
 sum(case when t.scs is not null and t.ccs is null then 1 else 0 end) store_only
,sum(case when t.scs is null and t.ccs is not null then 1 else 0 end) catalog_only
,sum(case when t.scs is not null and t.ccs is not null then 1 else 0 end) store_and_catalog
from (
 select ssci.customer_sk scs, csci.customer_sk ccs
 from ssci left join csci on (ssci.customer_sk=csci.customer_sk and ssci.item_sk=csci.item_sk)
 union all
 select ssci.customer_sk scs, csci.customer_sk ccs
 from ssci right join csci on (ssci.customer_sk=csci.customer_sk and ssci.item_sk=csci.item_sk)
 where ssci.customer_sk is null
) t`)
	default:
		return sql
	}
}
