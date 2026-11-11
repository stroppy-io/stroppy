package dsqgen

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed qualification.json
var qualificationJSON []byte

// Qualification renders the specification's fixed substitutions, not an RNG stream.
func Qualification(dialect Dialect) (*Result, error) {
	var document struct {
		Queries map[string]map[string][]string `json:"queries"`
	}
	if err := json.Unmarshal(qualificationJSON, &document); err != nil {
		return nil, err
	}
	templates, err := LoadTemplates()
	if err != nil {
		return nil, err
	}
	result := &Result{}
	for i, template := range templates {
		parameters, ok := document.Queries[template.Name]
		if !ok {
			return nil, fmt.Errorf("qualification parameters missing for %s", template.Name)
		}
		env := make(map[string][]cell, len(parameters))
		for name, values := range parameters {
			for _, value := range values {
				env[name] = append(env[name], strCell(value))
			}
		}
		sql := dialect.render(template, env, i+1, 0, 0)
		sql = qualificationTransforms(dialect.Name, template.Name, sql)
		if refRe.MatchString(sql) {
			return nil, fmt.Errorf("unresolved qualification parameter in %s: %s", template.Name, strings.Join(refRe.FindAllString(sql, -1), ", "))
		}
		result.Queries = append(result.Queries, Query{Name: template.Name, SQL: sql})
	}
	return result, nil
}

func QualificationSQL(dialect Dialect, result *Result) string {
	var output strings.Builder
	fmt.Fprintf(&output, "-- TPC-DS 4.0.0 qualification queries for %s.\n", dialect.Name)
	output.WriteString("-- Fixed substitutions: specification Appendix B, dsqgen/qualification.json.\n")
	output.WriteString("-- Regenerate with make gen-tpcds-qualification.\n\n")
	for _, query := range result.Queries {
		statements := strings.Split(strings.TrimSuffix(strings.TrimSpace(query.SQL), ";"), ";")
		for i, statement := range statements {
			name := strings.Replace(query.Name, "query", "query_", 1)
			if len(statements) > 1 {
				name += "_" + string(rune('a'+i))
			}
			fmt.Fprintf(&output, "--= %s\n", name)
			for _, line := range strings.Split(strings.TrimSpace(statement), "\n") {
				output.WriteString(strings.TrimRight(strings.ReplaceAll(line, "\t", "    "), " "))
				output.WriteByte('\n')
			}
			output.WriteString(";\n\n")
		}
	}
	return strings.TrimSpace(output.String()) + "\n"
}

var qualificationAverage = regexp.MustCompile(`(?is)where ctr1\.ctr_total_return\s*>\s*\(select avg\(ctr_total_return\)\*1\.2\s+from customer_total_return ctr2\s+where ctr1\.(ctr_store_sk|ctr_state)\s*=\s*ctr2\.(?:ctr_store_sk|ctr_state)\)`)

func qualificationTransforms(dialect, query, sql string) string {
	// A materialized return CTE has no statistics; correlated averages rescan it per customer.
	if query == "query1" || query == "query30" || query == "query81" {
		match := qualificationAverage.FindStringSubmatch(sql)
		if len(match) == 2 {
			key := match[1]
			cte := ",ctr_avg as (select " + key + ", avg(ctr_total_return)*1.2 avg_return from customer_total_return group by " + key + ")\n"
			sql = strings.Replace(sql, "select  c_customer_id", cte+"select  c_customer_id", 1)
			sql = qualificationAverage.ReplaceAllString(sql,
				",ctr_avg where ctr1.ctr_total_return > ctr_avg.avg_return and ctr_avg."+key+" = ctr1."+key)
		}
	}
	if query == "query6" && dialect == "postgres" {
		sql = decorrelateQ6(sql)
	}
	if dialect == "postgres" {
		switch query {
		case "query18":
			sql = strings.ReplaceAll(sql, "order by ca_country,", "order by ca_country collate \"C\",")
			sql = strings.ReplaceAll(sql, "ca_state, \n        ca_county,", "ca_state collate \"C\", \n        ca_county collate \"C\",")
		case "query21":
			sql = strings.ReplaceAll(sql, "inv_after / inv_before", "cast(inv_after as numeric) / inv_before")
		case "query34", "query73":
			sql = strings.ReplaceAll(sql, "household_demographics.hd_dep_count/", "cast(household_demographics.hd_dep_count as numeric)/")
		case "query83":
			for _, channel := range []string{"sr", "cr", "wr"} {
				sql = strings.ReplaceAll(sql, channel+"_item_qty/(", "cast("+channel+"_item_qty as numeric)/(")
			}
		}
	}
	if query == "query78" && dialect == "postgres" {
		sql = strings.ReplaceAll(sql, "round(ss_qty/", "round(cast(ss_qty as numeric)/")
	}
	if query == "query84" {
		// Oracle qualification output preserves the fixed-width surname before concatenation.
		sql = strings.ReplaceAll(sql, "coalesce(c_last_name,'')", "rpad(coalesce(c_last_name,''),30,' ')")
	}
	if query == "query72" && (dialect == "mysql" || dialect == "mariadb") {
		sql = strings.ReplaceAll(sql, "d1.d_date + 5", "d1.d_date + interval 5 day")
	}
	return sql
}
