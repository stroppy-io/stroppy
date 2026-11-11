package tpcds

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestQualificationPortListsMatchManifest(t *testing.T) {
	manifestData, err := published.ReadFile("dsqgen/qualification.json")
	if err != nil {
		t.Fatal(err)
	}

	var manifest struct {
		Queries map[string]map[string][]string `json:"queries"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}

	quoted := regexp.MustCompile(`'([^']*)'`)

	for _, file := range []string{"ydb.sql", "pico.sql"} {
		data, err := files.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		sql := bench.ParseSQL(string(data))
		for _, test := range []struct {
			query, parameter, pattern string
		}{
			{"8", "ZIP", `(?is)'24128'.*?'35576'`},
			{"13", "STATE", `(?is)ca_state\s+in\s*\([^)]*\)`},
			{"18", "STATE", `(?is)ca_state\s+in\s*\([^)]*\)`},
			{"20", "CATEGORY", `(?is)i_category\s+in\s*\([^)]*\)`},
			{"98", "CATEGORY", `(?is)i_category\s+in\s*\([^)]*\)`},
		} {
			body, ok := sql.Query("", "query_"+test.query)
			if !ok {
				t.Fatalf("%s missing query %s", file, test.query)
			}

			var got []string

			for _, list := range regexp.MustCompile(test.pattern).FindAllString(body, -1) {
				for _, value := range quoted.FindAllStringSubmatch(list, -1) {
					got = append(got, value[1])
				}
			}

			want := manifest.Queries["query"+test.query][test.parameter]
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("%s query %s %s differs from qualification manifest", file, test.query, test.parameter)
			}
		}
	}
}

func TestQualificationPortExpressions(t *testing.T) {
	for _, file := range []string{"ydb.sql", "pico.sql"} {
		data, err := files.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		sql := bench.ParseSQL(string(data))

		q66, _ := sql.Query("", "query_66")
		if count := strings.Count(q66, "cs_net_paid_inc_tax"); count != 12 ||
			regexp.MustCompile(`\bcs_net_paid\b`).MatchString(q66) {
			t.Errorf("%s query 66 catalog net aggregate differs from qualification", file)
		}

		q79, _ := sql.Query("", "query_79")
		if !strings.Contains(q79, "hd_vehicle_count > 2") {
			t.Errorf("%s query 79 vehicle threshold differs from qualification", file)
		}

		q88, _ := sql.Query("", "query_88")
		for _, value := range []string{"4", "2", "0"} {
			if strings.Count(q88, "hd_dep_count = "+value) != 8 ||
				strings.Count(q88, "hd_vehicle_count<="+value+"+2") != 8 {
				t.Errorf("%s query 88 demographic %s differs from qualification", file, value)
			}
		}

		q35, _ := sql.Query("", "query_35")
		if strings.Contains(q35, "stddev_samp") || strings.Count(strings.ToLower(q35), "max(") != 3 {
			t.Errorf("%s query 35 must return three qualification maxima", file)
		}
	}
}
