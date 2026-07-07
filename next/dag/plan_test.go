package dag

import (
	"strings"
	"testing"
)

func TestPlanRendering(t *testing.T) {
	g := NewGraph().
		Add(okNode("a")).
		Add(afterNode("b", "a")).
		Add(&Node{ID: "c", Run: okRun, AfterAny: []string{"a"}}).
		Add(&Node{ID: "cleanup", Run: okRun, OnFailure: []string{"a"}, Failure: SkipDependents})

	b, err := g.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	text := PlanText(b)
	for _, id := range []string{"a", "b", "c", "cleanup"} {
		if !strings.Contains(text, id) {
			t.Fatalf("PlanText missing %q:\n%s", id, text)
		}
	}

	if !strings.Contains(text, "policy="+SkipDependents.String()) {
		t.Fatalf("PlanText missing policy annotation:\n%s", text)
	}

	dot := PlanDOT(b)
	if !strings.HasPrefix(dot, "digraph dag {") {
		t.Fatalf("PlanDOT missing header:\n%s", dot)
	}

	if !strings.Contains(dot, `"a" -> "b"`) {
		t.Fatalf("PlanDOT missing a->b edge:\n%s", dot)
	}

	if !strings.Contains(dot, `"a" -> "cleanup"`) || !strings.Contains(dot, "style=dashed") {
		t.Fatalf("PlanDOT missing onFailure edge:\n%s", dot)
	}
}

func TestPlanEmptyGraph(t *testing.T) {
	b, err := NewGraph().Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if PlanText(b) != "" {
		t.Fatalf("PlanText of empty graph = %q, want empty", PlanText(b))
	}

	dot := PlanDOT(b)
	if !strings.HasPrefix(dot, "digraph dag {") || !strings.HasSuffix(dot, "}\n") {
		t.Fatalf("PlanDOT of empty graph malformed:\n%s", dot)
	}
}
