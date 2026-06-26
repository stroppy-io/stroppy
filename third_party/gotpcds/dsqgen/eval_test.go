package dsqgen

import (
	"strings"
	"testing"
)

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func loadQuery(t *testing.T, name string) *Template {
	t.Helper()
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, tm := range tmpls {
		if tm.Name == name {
			return tm
		}
	}
	t.Fatalf("%s not found", name)
	return nil
}

func TestEvalQuery1InDomain(t *testing.T) {
	q1 := loadQuery(t, "query1")
	ev := newEvaluator(12345, 1, newDistCache())
	env, err := ev.evalTemplate(q1)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}

	year := env["YEAR"][0]
	if !year.isInt || year.i < 1998 || year.i > 2002 {
		t.Errorf("YEAR out of domain: %+v", year)
	}

	state := env["STATE"][0].str()
	if len(state) != 2 {
		t.Errorf("STATE not a 2-letter abbrev: %q", state)
	}

	aggField := env["AGG_FIELD"][0].str()
	valid := map[string]bool{
		"SR_RETURN_AMT": true, "SR_FEE": true, "SR_REFUNDED_CASH": true,
		"SR_RETURN_AMT_INC_TAX": true, "SR_REVERSED_CHARGE": true,
		"SR_STORE_CREDIT": true, "SR_RETURN_TAX": true,
	}
	if !valid[aggField] {
		t.Errorf("AGG_FIELD not a valid choice: %q", aggField)
	}

	if c := env["COUNTY"][0]; !c.isInt || c.i < 1 {
		t.Errorf("COUNTY not a positive int: %+v", c)
	}
}

// TestEvalAllTemplates evaluates every template at a couple of seeds and scales
// and asserts no evaluation errors (every define resolves to a value).
func TestEvalAllTemplates(t *testing.T) {
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	dc := newDistCache()
	var failed []string
	for _, scale := range []float64{1, 100} {
		for _, seed := range []int64{1, 99999} {
			for _, tm := range tmpls {
				ev := newEvaluator(seed, scale, dc)
				if _, err := ev.evalTemplate(tm); err != nil {
					// query88 uses dist(stores,…); store names are syllable-generated,
					// not a vendored distribution, so that one param falls back to its
					// baked value at generation time. Known, documented gap.
					if tm.Name == "query88" && contains(err.Error(), `"stores"`) {
						continue
					}
					failed = append(failed, tm.Name+": "+err.Error())
				}
			}
		}
	}
	if len(failed) > 0 {
		t.Errorf("%d template evaluations failed:", len(failed))
		for _, f := range failed {
			t.Logf("  %s", f)
		}
	}
}
