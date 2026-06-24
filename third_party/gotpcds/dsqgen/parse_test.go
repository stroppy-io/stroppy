package dsqgen

import "testing"

// TestParseAllTemplates parses every vendored template and asserts the headers
// are well-formed (no parse errors across all 99).
func TestParseAllTemplates(t *testing.T) {
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	if len(tmpls) != 99 {
		t.Fatalf("expected 99 templates, got %d", len(tmpls))
	}
	for _, tm := range tmpls {
		if tm.Body == "" {
			t.Errorf("%s: empty body", tm.Name)
		}
	}
}

// TestParseQuery1 pins the structure of query1's defines (the canonical example).
func TestParseQuery1(t *testing.T) {
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var q1 *Template
	for _, tm := range tmpls {
		if tm.Name == "query1" {
			q1 = tm
		}
	}
	if q1 == nil {
		t.Fatal("query1 not found")
	}

	byName := map[string]Define{}
	for _, d := range q1.Defines {
		byName[d.Name] = d
	}

	county, ok := byName["COUNTY"]
	if !ok {
		t.Fatal("COUNTY define missing")
	}
	if county.RHS.Kind != ExprCall || county.RHS.Func != "random" {
		t.Errorf("COUNTY: want random(...) call, got kind=%d func=%q", county.RHS.Kind, county.RHS.Func)
	}

	state, ok := byName["STATE"]
	if !ok {
		t.Fatal("STATE define missing")
	}
	if state.RHS.Func != "distmember" {
		t.Errorf("STATE: want distmember, got %q", state.RHS.Func)
	}
	if len(state.Deps) != 1 || state.Deps[0] != "COUNTY" {
		t.Errorf("STATE deps: want [COUNTY], got %v", state.Deps)
	}

	agg, ok := byName["AGG_FIELD"]
	if !ok {
		t.Fatal("AGG_FIELD define missing")
	}
	if agg.RHS.Func != "text" {
		t.Errorf("AGG_FIELD: want text, got %q", agg.RHS.Func)
	}
	if len(agg.RHS.Args) == 0 || agg.RHS.Args[0].Kind != ExprTuple {
		t.Errorf("AGG_FIELD: want tuple args, got %+v", agg.RHS.Args)
	}
}
