package bench

import (
	"hash/fnv"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/next/dag"
	"github.com/stroppy-io/stroppy/next/metrics"
)

func TestStepIDStableAndPure(t *testing.T) {
	// Stable across calls.
	a, b := stepID("load"), stepID("load")
	if a != b {
		t.Fatal("stepID not stable across calls")
	}
	// Distinct names -> distinct ids (no collision among these).
	names := []string{"drop", "create", "load", "workload", "check", "cleanup"}
	seen := make(map[uint32]string)
	for _, n := range names {
		id := stepID(n)
		if prev, dup := seen[id]; dup {
			t.Fatalf("stepID collision: %q and %q both hash to %d", prev, n, id)
		}
		seen[id] = n
	}
	// Contract: id is the 32-bit FNV-1a of the name, independent of anything else.
	h := fnv.New32a()
	_, _ = h.Write([]byte("workload"))
	if stepID("workload") != h.Sum32() {
		t.Fatal("stepID must equal FNV-1a-32 of the name")
	}
}

func TestBuildStepFilter(t *testing.T) {
	steps := buildStepFilter("a, b", "")
	if steps == nil || !steps("a") || !steps("b") || steps("c") {
		t.Fatal("-steps filter should admit only a,b")
	}
	no := buildStepFilter("", "c,d")
	if no == nil || !no("a") || no("c") || no("d") {
		t.Fatal("-no-steps filter should admit all but c,d")
	}
	if buildStepFilter("", "") != nil {
		t.Fatal("no selection should produce a nil filter")
	}
}

// okHandler is a no-op handler for translation tests.
type okHandler struct{}

func (okHandler) Init(*VU) error  { return nil }
func (okHandler) Iter(*VU) error  { return nil }
func (okHandler) Close(*VU) error { return nil }

// diamondTest builds a→{b,c}→d with an If on c.
func diamondTest() *Test {
	return &Test{
		Name:    "diamond",
		Drivers: []DriverSlot{{Name: "main", Kind: "noop"}},
		Steps: []*StepDef{
			Step("a", okHandler{}),
			Step("b", okHandler{}).After("a"),
			Step("c", okHandler{}).After("a").If(func(*Run) bool { return true }),
			Step("d", okHandler{}).After("b", "c"),
		},
	}
}

func TestBuildGraphEdges(t *testing.T) {
	tst := diamondTest()
	run := &Run{test: tst, slots: []slotSpec{{name: "main", kind: "noop"}}}
	reg := metrics.NewRegistry()
	drivers, _ := buildDrivers(run.slots)
	built, execs, err := buildGraph(tst, run, 0, reg, drivers, run.slots, nil)
	if err != nil {
		t.Fatalf("buildGraph: %v", err)
	}
	if len(execs) != 4 {
		t.Fatalf("built %d executors, want 4", len(execs))
	}
	plan := dag.PlanText(built)
	for _, want := range []string{
		"b after=[a]",
		"d after=[b,c]",
		"c after=[a] if",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
}

func TestBuildGraphUnknownSlot(t *testing.T) {
	tst := &Test{
		Name:    "x",
		Drivers: []DriverSlot{{Name: "main", Kind: "noop"}},
		Steps:   []*StepDef{Step("a", okHandler{}).Uses("nope")},
	}
	run := &Run{test: tst, slots: resolveSlots(tst.Drivers, envMap(nil))}
	drivers, _ := buildDrivers(run.slots)
	if _, _, err := buildGraph(tst, run, 0, metrics.NewRegistry(), drivers, run.slots, nil); err == nil {
		t.Fatal("expected an error for a step using an unknown slot")
	}
}

func TestBuildGraphDuplicateStepRejected(t *testing.T) {
	tst := &Test{
		Name:    "dup",
		Drivers: []DriverSlot{{Name: "main", Kind: "noop"}},
		Steps:   []*StepDef{Step("a", okHandler{}), Step("a", okHandler{})},
	}
	run := &Run{test: tst, slots: resolveSlots(tst.Drivers, envMap(nil))}
	drivers, _ := buildDrivers(run.slots)
	if _, _, err := buildGraph(tst, run, 0, metrics.NewRegistry(), drivers, run.slots, nil); err == nil {
		t.Fatal("expected a duplicate-id error from the dag builder")
	}
}

func TestProbeGolden(t *testing.T) {
	tst := &Test{
		Name:    "golden",
		Seed:    42,
		Drivers: []DriverSlot{{Name: "main", Kind: "noop", URL: "noop://"}},
		Steps: []*StepDef{
			Step("setup", okHandler{}).OnErr(Silent),
			Step("run", okHandler{}).
				Closed(4, 3*time.Second).
				After("setup").
				If(func(*Run) bool { return true }).
				Uses("main"),
		},
	}
	schema, err := parseOptions(tst.Opts, envMap(nil))
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	slots := resolveSlots(tst.Drivers, envMap(nil))
	var sb strings.Builder
	if err := writeProbe(&sb, buildProbe(tst, tst.Seed, schema, slots)); err != nil {
		t.Fatalf("writeProbe: %v", err)
	}
	got := sb.String()
	want := `{
  "name": "golden",
  "seed": 42,
  "options": null,
  "drivers": [
    {
      "name": "main",
      "kind": "noop",
      "url": "noop://"
    }
  ],
  "steps": [
    {
      "name": "setup",
      "executor": {
        "kind": "once"
      },
      "if": false,
      "onErr": "silent",
      "retryMaxAttempts": 0,
      "uses": "",
      "usesSlot": 0
    },
    {
      "name": "run",
      "executor": {
        "kind": "closed",
        "vus": 4,
        "duration": "3s"
      },
      "after": [
        "setup"
      ],
      "if": true,
      "onErr": "log",
      "retryMaxAttempts": 0,
      "uses": "main",
      "usesSlot": 0
    }
  ]
}
`
	if got != want {
		t.Fatalf("probe JSON mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
