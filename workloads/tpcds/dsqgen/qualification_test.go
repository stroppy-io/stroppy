package dsqgen

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestQualificationComplete(t *testing.T) {
	for _, dialect := range []Dialect{Postgres, MySQL} {
		result, err := Qualification(dialect)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Queries) != 99 || len(result.Skipped) != 0 {
			t.Fatalf("%s: got %d queries, %v skips", dialect.Name, len(result.Queries), result.Skipped)
		}
		sql := QualificationSQL(dialect, result)
		if strings.Count(sql, "--= query_") != 103 || refRe.MatchString(sql) {
			t.Fatalf("%s qualification set incomplete", dialect.Name)
		}
		if dialect.Name == "mysql" && (strings.Contains(sql, "full outer join") || strings.Contains(sql, "||")) {
			t.Fatal("qualification retained unsupported MySQL syntax")
		}
		for _, query := range result.Queries {
			switch query.Name {
			case "query1", "query30", "query81":
				if !strings.Contains(query.SQL, "avg_return") {
					t.Fatalf("%s lost grouped-average rewrite", query.Name)
				}
			case "query96":
				if !strings.Contains(query.SQL, "t_hour = 20") || !strings.Contains(query.SQL, "hd_dep_count = 7") {
					t.Fatalf("wrong q96 qualification substitutions: %s", query.SQL)
				}
			}
		}
		repeated, err := Qualification(dialect)
		if err != nil || !reflect.DeepEqual(result, repeated) {
			t.Fatalf("qualification values changed between calls: %v", err)
		}
	}
}

func TestQualificationManifest(t *testing.T) {
	var document struct {
		Version             string                         `json:"version"`
		SpecificationSHA256 string                         `json:"specificationSha256"`
		Queries             map[string]map[string][]string `json:"queries"`
	}
	if err := json.Unmarshal(qualificationJSON, &document); err != nil {
		t.Fatal(err)
	}
	if document.Version != "4.0.0" || document.SpecificationSHA256 != "f66af4b53099680669bfa49cafeb59325373edaafd74e26093f3f1334c8695fe" {
		t.Fatal("qualification source identity changed")
	}
	if len(document.Queries) != 99 || len(document.Queries["query8"]["ZIP"]) != 400 {
		t.Fatal("qualification manifest lost queries or ZIP substitutions")
	}
	for query, parameters := range document.Queries {
		for name, values := range parameters {
			if len(values) == 0 {
				t.Fatalf("%s.%s has no qualification value", query, name)
			}
		}
	}
}
