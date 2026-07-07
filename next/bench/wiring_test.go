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
		Build: func(*Run) []*StepDef {
			return []*StepDef{
				Step("a", okHandler{}),
				Step("b", okHandler{}).After("a"),
				Step("c", okHandler{}).After("a").If(func(*Run) bool { return true }),
				Step("d", okHandler{}).After("b", "c"),
			}
		},
	}
}

func TestBuildGraphEdges(t *testing.T) {
	tst := diamondTest()
	run := &Run{test: tst, slots: []slotSpec{{name: "main", kind: "noop"}}}
	reg := metrics.NewRegistry()
	drivers, _ := buildDrivers(run.slots)
	built, execs, err := buildGraph(buildSteps(tst, run), run, 0, reg, drivers, run.slots, nil)
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
		Build:   func(*Run) []*StepDef { return []*StepDef{Step("a", okHandler{}).Uses("nope")} },
	}
	run := &Run{test: tst, slots: resolveSlots(tst.Drivers, "", "", envMap(nil))}
	drivers, _ := buildDrivers(run.slots)
	if _, _, err := buildGraph(buildSteps(tst, run), run, 0, metrics.NewRegistry(), drivers, run.slots, nil); err == nil {
		t.Fatal("expected an error for a step using an unknown slot")
	}
}

func TestBuildGraphDuplicateStepRejected(t *testing.T) {
	tst := &Test{
		Name:    "dup",
		Drivers: []DriverSlot{{Name: "main", Kind: "noop"}},
		Build:   func(*Run) []*StepDef { return []*StepDef{Step("a", okHandler{}), Step("a", okHandler{})} },
	}
	run := &Run{test: tst, slots: resolveSlots(tst.Drivers, "", "", envMap(nil))}
	drivers, _ := buildDrivers(run.slots)
	if _, _, err := buildGraph(buildSteps(tst, run), run, 0, metrics.NewRegistry(), drivers, run.slots, nil); err == nil {
		t.Fatal("expected a duplicate-id error from the dag builder")
	}
}

func TestProbeGolden(t *testing.T) {
	tst := &Test{
		Name:    "golden",
		Seed:    "42",
		Drivers: []DriverSlot{{Name: "main", Kind: "noop", URL: "noop://"}},
		Build: func(*Run) []*StepDef {
			return []*StepDef{
				Step("setup", okHandler{}).OnErr(ModeSilent),
				Step("run", okHandler{}).
					Closed(4, 3*time.Second).
					After("setup").
					If(func(*Run) bool { return true }).
					Uses("main"),
			}
		},
	}
	// Mirror runMain's probe path: bags -> paramSet -> standard params ->
	// struct-tag bridge -> schema -> probe.
	set := newParamSet(nil, envMap(nil), nil)
	seedP, drvURL, drvKind := registerStandardParams(tst, set)
	if err := parseOptions(tst.Opts, set); err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if err := set.Err(); err != nil {
		t.Fatalf("param set: %v", err)
	}
	rootSeed, err := resolveSeed(seedP.Value(), tst.Seed)
	if err != nil {
		t.Fatalf("resolveSeed: %v", err)
	}
	slots := resolveSlots(tst.Drivers, drvURL.Value(), drvKind.Value(), envMap(nil))
	run := &Run{test: tst, seed: rootSeed, slots: slots}
	var sb strings.Builder
	if err := writeProbe(&sb, buildProbe(tst, buildSteps(tst, run), rootSeed, set.Schema(), slots, run)); err != nil {
		t.Fatalf("writeProbe: %v", err)
	}
	got := sb.String()
	want := `{
  "name": "golden",
  "seed": 42,
  "params": [
    {
      "name": "seed",
      "env": "SEED",
      "flag": "--seed",
      "config": "seed",
      "type": "string",
      "help": "run root seed: auto|now (random per run), fixed|canonical (this test's spec seed), or a uint64",
      "default": "42",
      "current": "42",
      "source": "default"
    },
    {
      "name": "driver.url",
      "env": "STROPPY_DRIVER_URL",
      "flag": "--driver.url",
      "config": "driver.url",
      "type": "string",
      "help": "slot-0 database URL",
      "default": "noop://",
      "current": "noop://",
      "source": "default"
    },
    {
      "name": "driver.kind",
      "env": "STROPPY_DRIVER_KIND",
      "flag": "--driver.kind",
      "config": "driver.kind",
      "type": "string",
      "help": "slot-0 driver kind (pg|noop)",
      "default": "noop",
      "current": "noop",
      "source": "default"
    }
  ],
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
