package bench

import (
	"testing"
)

// TestHistogramCounterForwardRef: declaring an instrument returns a handle
// whose underlying metrics handle is not yet assigned (D6 wires phase-3
// registration); Handle panics with a clear message until then.
func TestHistogramCounterForwardRef(t *testing.T) {
	set := newParamSet(nil, envMap(nil), nil)
	_, _, _, _ = registerStandardParams(&Test{Name: "x"}, set)
	run := &Run{test: &Test{Name: "x"}, getenv: envMap(nil)}
	d := newDef(&Test{Name: "x"}, set, run)

	h := d.Histogram("servicetime", Tag("tx"))
	if h.Name() != "servicetime" {
		t.Fatalf("hist name = %q, want servicetime", h.Name())
	}
	if len(d.instruments) != 1 || d.instruments[0].kind != kindHistogram {
		t.Fatalf("histogram not recorded: %+v", d.instruments)
	}
	if len(d.instruments[0].tags) != 1 || d.instruments[0].tags[0].name != "tx" {
		t.Fatalf("tx tag not recorded: %+v", d.instruments[0].tags)
	}
	if !panics(func() { h.Handle() }) {
		t.Fatal("Histogram.Handle should panic before phase-3 registration (D6)")
	}

	c := d.Counter("errors", Tag("tx"), Tag("step"))
	if c.Name() != "errors" {
		t.Fatalf("counter name = %q, want errors", c.Name())
	}
	if len(d.instruments) != 2 || d.instruments[1].kind != kindCounter {
		t.Fatalf("counter not recorded: %+v", d.instruments)
	}
	if len(d.instruments[1].tags) != 2 {
		t.Fatalf("two tags not recorded: %+v", d.instruments[1].tags)
	}
	if !panics(func() { c.Handle() }) {
		t.Fatal("Counter.Handle should panic before phase-3 registration (D6)")
	}
}

// TestVariantSelection: a variant with a subset of steps admits only those
// steps; an unknown variant name is rejected; an empty step set means all.
func TestVariantSelection(t *testing.T) {
	set := newParamSet(nil, envMap(nil), nil)
	_, _, _, _ = registerStandardParams(&Test{Name: "x"}, set)
	run := &Run{test: &Test{Name: "x"}, getenv: envMap(nil)}
	d := newDef(&Test{Name: "x"}, set, run)

	a := d.Step("a", okHandler{})
	b := d.Step("b", okHandler{})
	d.Step("c", okHandler{})
	d.Variant("full")
	d.Variant("ab", a, b)

	if got := names(selectVariantMust(t, d, "full")); len(got) != 3 {
		t.Fatalf("full variant should admit all 3, got %v", got)
	}
	if got := names(selectVariantMust(t, d, "ab")); len(got) != 2 {
		t.Fatalf("ab variant should admit 2, got %v", got)
	}
	ab := selectVariantMust(t, d, "ab")
	active := variantActiveSet(t, d, "ab")
	for _, sd := range ab {
		if !active[sd.name] {
			t.Fatalf("active set missing %q", sd.name)
		}
	}
	if active["c"] {
		t.Fatal("ab variant must not admit c")
	}

	// Edges to excluded steps drop: b.After(c) -> c is excluded in "ab".
	b.After("c")
	pruneEdges(ab, active)
	for _, e := range b.after {
		if e == "c" {
			t.Fatal("edge to excluded step c should have been pruned")
		}
	}

	if _, _, err := selectVariantSteps(d, "nope"); err == nil {
		t.Fatal("unknown variant should be rejected")
	}
}

// TestVariantEmptyMeansAll: a variant declared with no steps admits every
// declared step (D3b: empty = all).
func TestVariantEmptyMeansAll(t *testing.T) {
	set := newParamSet(nil, envMap(nil), nil)
	_, _, _, _ = registerStandardParams(&Test{Name: "x"}, set)
	run := &Run{test: &Test{Name: "x"}, getenv: envMap(nil)}
	d := newDef(&Test{Name: "x"}, set, run)
	d.Step("a", okHandler{})
	d.Step("b", okHandler{})
	d.Variant("full") // no steps = all
	got := selectVariantMust(t, d, "full")
	if len(got) != 2 {
		t.Fatalf("empty variant should admit all 2, got %v", got)
	}
}

// TestVariantFullRequired: a Define that declares variants but none named
// "full" is rejected (D5: the standard variant param defaults to "full").
func TestVariantFullRequired(t *testing.T) {
	set := newParamSet(nil, envMap(nil), nil)
	_, _, _, _ = registerStandardParams(&Test{Name: "x"}, set)
	run := &Run{test: &Test{Name: "x"}, getenv: envMap(nil)}
	d := newDef(&Test{Name: "x"}, set, run)
	a := d.Step("a", okHandler{})
	d.Variant("only", a) // no "full"
	if _, _, err := selectVariantSteps(d, "only"); err == nil {
		t.Fatal("expected an error when no variant is named \"full\"")
	}
}

// TestNoVariantsAdmitsAll: a Define that declares steps but no variant gets an
// implicit "every declared step" active set under any chosen name (the chosen
// name is only validated when variants exist).
func TestNoVariantsAdmitsAll(t *testing.T) {
	set := newParamSet(nil, envMap(nil), nil)
	_, _, _, _ = registerStandardParams(&Test{Name: "x"}, set)
	run := &Run{test: &Test{Name: "x"}, getenv: envMap(nil)}
	d := newDef(&Test{Name: "x"}, set, run)
	d.Step("a", okHandler{})
	d.Step("b", okHandler{})
	got := selectVariantMust(t, d, "full")
	if len(got) != 2 {
		t.Fatalf("no-variant Define should admit all 2, got %v", got)
	}
}

// TestSkippableMarker: Skippable sets the guardrail flag the probe reports.
func TestSkippableMarker(t *testing.T) {
	sd := Step("x", okHandler{})
	if sd.skippable {
		t.Fatal("step should default to non-skippable")
	}
	sd.Skippable()
	if !sd.skippable {
		t.Fatal("Skippable() should set the flag")
	}
}

// TestDriverSlot0Override: slot 0's kind/url honor the operator's standard
// driver.kind/driver.url overrides over the declared default.
func TestDriverSlot0Override(t *testing.T) {
	cli := map[string]string{"driver.kind": "noop"}
	set := newParamSet(cli, envMap(nil), nil)
	_, drvURL, drvKind, _ := registerStandardParams(&Test{Name: "x"}, set)
	run := &Run{test: &Test{Name: "x"}, getenv: envMap(nil), stdDriverURL: drvURL, stdDriverKind: drvKind}
	d := newDef(&Test{Name: "x"}, set, run)
	d.Driver("main", "pg", WithURL("postgres://x"))
	if got := run.slots[0].kind; got != "noop" {
		t.Fatalf("slot 0 kind = %q, want noop (operator override)", got)
	}
	if got := run.slots[0].url; got != "postgres://x" {
		t.Fatalf("slot 0 url = %q, want declared default (no override)", got)
	}
}

func selectVariantMust(t *testing.T, d *Def, chosen string) []*StepDef {
	t.Helper()
	steps, _, err := selectVariantSteps(d, chosen)
	if err != nil {
		t.Fatalf("selectVariantSteps(%q): %v", chosen, err)
	}
	return steps
}

func variantActiveSet(t *testing.T, d *Def, chosen string) map[string]bool {
	t.Helper()
	_, active, err := selectVariantSteps(d, chosen)
	if err != nil {
		t.Fatalf("selectVariantSteps(%q): %v", chosen, err)
	}
	return active
}

func names(steps []*StepDef) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.name
	}
	return out
}

func panics(fn func()) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = true
		}
	}()
	fn()
	return
}
