package dsqgen

import (
	"fmt"
	"strings"
)

func mariaDBTransform(query, sql string) string {
	sql = mysqlTransform(query, sql)
	if query == "query72" {
		sql = strings.ReplaceAll(sql, "d1.d_date + 5", "d1.d_date + interval 5 day")
	}
	if query == "query27" {
		return mariaDBGrouping(sql, "i_item_id", "s_state", "", "")
	}
	switch query {
	case "query36":
		return mariaDBGrouping(sql, "i_category", "i_class", "sum(ss_net_profit)/sum(ss_ext_sales_price)", "gross_margin")
	case "query70":
		return mariaDBGrouping(sql, "s_state", "s_county", "sum(ss_net_profit)", "total_sum")
	case "query86":
		return mariaDBGrouping(sql, "i_category", "i_class", "sum(ws_net_paid)", "total_sum")
	}

	parts := strings.Split(sql, ";")
	for i, part := range parts {
		positions := topLevelClauses(part)
		selectAt, selectOK := positions["select"]
		groupAt, groupOK := positions["group by"]
		orderAt, orderOK := positions["order by"]
		if !selectOK || !groupOK || !orderOK || !strings.Contains(strings.ToLower(part[groupAt:orderAt]), "with rollup") {
			continue
		}
		// Sorting and limiting must happen after MariaDB has produced all rollup rows.
		parts[i] = part[:selectAt] + "select * from (" + strings.TrimSpace(part[selectAt:orderAt]) +
			") rollup_result\n" + part[orderAt:]
	}
	return strings.Join(parts, ";")
}

func mariaDBGrouping(sql, first, second, aggregate, alias string) string {
	positions := topLevelClauses(sql)
	from := sql[positions["from"]:positions["group by"]]
	group := []string{first + ", " + second, first, ""}
	branches := make([]string, 0, len(group))
	for level, columns := range group {
		left, right := first, second
		if level > 0 {
			right = "NULL"
		}
		if level > 1 {
			left = "NULL"
		}
		var projection string
		if aggregate == "" {
			projection = fmt.Sprintf("%s as %s, %s as %s, %d as g_state, "+
				"avg(ss_quantity) agg1, avg(ss_list_price) agg2, avg(ss_coupon_amt) agg3, avg(ss_sales_price) agg4",
				left, first, right, second, min(level, 1))
		} else {
			parent := "NULL"
			if level == 0 {
				parent = first
			}
			projection = fmt.Sprintf("%s as %s, %s as %s, %s as %s, %d as lochierarchy, %s as parent_key",
				aggregate, alias, left, first, right, second, level, parent)
		}
		branch := "select " + projection + "\n" + from
		if columns != "" {
			branch += "\ngroup by " + columns
		} else {
			branch += "\nhaving count(*) > 0"
		}
		branches = append(branches, branch)
	}
	// Explicit levels preserve the difference between real NULL keys and subtotal NULLs.
	union := "(" + strings.Join(branches, "\nunion all\n") + ") rollup_levels"
	if aggregate == "" {
		return "select " + first + ", " + second + ", g_state, agg1, agg2, agg3, agg4 from " +
			union + "\n" + strings.TrimSuffix(sql[positions["order by"]:], ";")
	}
	direction := "desc"
	if alias == "gross_margin" {
		direction = "asc"
	}
	order := strings.TrimSuffix(strings.TrimSpace(sql[positions["order by"]:]), ";")
	order = strings.ReplaceAll(order, "grouping("+first+")+grouping("+second+")", "lochierarchy")
	return fmt.Sprintf("select %s, %s, %s, lochierarchy, "+
		"rank() over (partition by lochierarchy, parent_key order by %s %s) as rank_within_parent\n"+
		"from %s\n%s", alias, first, second, alias, direction, union, order)
}

// Clause boundaries ignore window ordering, CTE bodies, and quoted text.
func topLevelClauses(sql string) map[string]int {
	positions := make(map[string]int)
	depth := 0
	var quote byte
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if quote != 0 {
			if ch == '\\' {
				i++
			} else if ch == quote {
				if i+1 < len(sql) && sql[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			quote = ch
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth != 0 || (i > 0 && isIdentByte(sql[i-1])) {
				continue
			}
			for _, clause := range []string{"select", "from", "group by", "order by", "limit"} {
				end := i + len(clause)
				if end <= len(sql) && strings.EqualFold(sql[i:end], clause) &&
					(end == len(sql) || !isIdentByte(sql[end])) {
					positions[clause] = i
				}
			}
		}
	}
	return positions
}
