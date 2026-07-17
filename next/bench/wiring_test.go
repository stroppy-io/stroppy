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

func TestBuildSkipFilter(t *testing.T) {
	declared := map[string]*StepDef{
		"a": Step("a", okHandler{}).Skippable(),
		"b": Step("b", okHandler{}).Skippable(),
		"c": Step("c", okHandler{}), // required (not Skippable)
	}

	// -skip=a,b admits all but a,b (both Skippable).
	f, err := buildSkipFilter("a, b", declared)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f == nil || !f("c") || f("a") || f("b") {
		t.Fatal("-skip filter should admit all but a,b")
	}

	// -skip on a required (non-Skippable) step is a hard error.
	if _, err := buildSkipFilter("c", declared); err == nil {
		t.Fatal("-skip on a non-skippable step should be a hard error")
	}

	// -skip on an undeclared name is a hard error (typo guard).
	if _, err := buildSkipFilter("ghost", declared); err == nil {
		t.Fatal("-skip on an undeclared step should be a hard error")
	}

	// Empty -skip produces a nil filter (no admission change).
	if got, err := buildSkipFilter("", declared); err != nil || got != nil {
		t.Fatal("empty -skip should produce a nil filter with no error")
	}
}

// TestValidateFullGraph: validateFullGraph catches typo edges and cycles in the
// FULL declared graph (every step, every author edge) before variant pruning
// silently drops them — D3b validate-full-then-prune.
func TestValidateFullGraph(t *testing.T) {
	t.Run("typo edge reported against the full graph", func(t *testing.T) {
		steps := []*StepDef{
			Step("a", okHandler{}),
			Step("b", okHandler{}).After("ghost"), // typo: ghost is not declared
		}
		if err := validateFullGraph(steps); err == nil {
			t.Fatal("validateFullGraph should report the typo edge to ghost")
		}
	})

	t.Run("cycle reported", func(t *testing.T) {
		steps := []*StepDef{
			Step("a", okHandler{}).After("b"),
			Step("b", okHandler{}).After("a"),
		}
		if err := validateFullGraph(steps); err == nil {
			t.Fatal("validateFullGraph should report the cycle")
		}
	})

	t.Run("valid graph accepted", func(t *testing.T) {
		steps := []*StepDef{
			Step("a", okHandler{}),
			Step("b", okHandler{}).After("a"),
		}
		if err := validateFullGraph(steps); err != nil {
			t.Fatalf("validateFullGraph rejected a valid graph: %v", err)
		}
	})

	t.Run("empty declaration accepted", func(t *testing.T) {
		if err := validateFullGraph(nil); err != nil {
			t.Fatalf("validateFullGraph(nil) = %v, want nil", err)
		}
	})
}

// okHandler is a no-op handler for translation tests.
type okHandler struct{}

func (okHandler) Init(*VU) error  { return nil }
func (okHandler) Iter(*VU) error  { return nil }
func (okHandler) Close(*VU) error { return nil }

// diamondDefine builds a→{b,c}→d with an If on c into d.
func diamondDefine(d *Def) error {
	d.Driver("main", "noop")
	d.Step("a", okHandler{})
	d.Step("b", okHandler{}).After("a")
	d.Step("c", okHandler{}).After("a").If(func(*Run) bool { return true })
	d.Step("d", okHandler{}).After("b", "c")
	d.Variant("full")
	return nil
}

func diamondTest() *Test {
	return &Test{Name: "diamond", Define: diamondDefine}
}

func TestBuildGraphEdges(t *testing.T) {
	tst := diamondTest()
	run := &Run{test: tst, slots: []slotSpec{{name: "main", kind: "noop"}}}
	d := newDef(tst, newParamSet(nil, envMap(nil), nil), run)
	if err := tst.Define(d); err != nil {
		t.Fatalf("Define: %v", err)
	}
	active, activeSet, err := selectVariantSteps(d, "full")
	if err != nil {
		t.Fatalf("selectVariantSteps: %v", err)
	}
	pruneEdges(active, activeSet)
	reg := metrics.NewRegistry()
	drivers, acq, _ := buildDrivers(run.slots)
	built, execs, err := buildGraph(active, run, 0, reg, drivers, acq, run.slots, nil)
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
		Name: "x",
		Define: func(d *Def) error {
			d.Driver("main", "noop")
			d.Step("a", okHandler{}).Uses("nope")
			return nil
		},
	}
	run := &Run{test: tst}
	d := newDef(tst, newParamSet(nil, envMap(nil), nil), run)
	if err := tst.Define(d); err != nil {
		t.Fatalf("Define: %v", err)
	}
	active, activeSet, err := selectVariantSteps(d, "full")
	if err != nil {
		t.Fatalf("selectVariantSteps: %v", err)
	}
	pruneEdges(active, activeSet)
	drivers, acq, _ := buildDrivers(run.slots)
	if _, _, err := buildGraph(active, run, 0, metrics.NewRegistry(), drivers, acq, run.slots, nil); err == nil {
		t.Fatal("expected an error for a step using an unknown slot")
	}
}

func TestBuildGraphDuplicateStepRejected(t *testing.T) {
	tst := &Test{
		Name: "dup",
		Define: func(d *Def) error {
			d.Driver("main", "noop")
			d.Step("a", okHandler{})
			d.Step("a", okHandler{})
			return nil
		},
	}
	run := &Run{test: tst}
	d := newDef(tst, newParamSet(nil, envMap(nil), nil), run)
	if err := tst.Define(d); err != nil {
		t.Fatalf("Define: %v", err)
	}
	active, activeSet, err := selectVariantSteps(d, "full")
	if err != nil {
		t.Fatalf("selectVariantSteps: %v", err)
	}
	pruneEdges(active, activeSet)
	drivers, acq, _ := buildDrivers(run.slots)
	if _, _, err := buildGraph(active, run, 0, metrics.NewRegistry(), drivers, acq, run.slots, nil); err == nil {
		t.Fatal("expected a duplicate-id error from the dag builder")
	}
}

func TestProbeGolden(t *testing.T) {
	tst := &Test{
		Name: "golden",
		Seed: "42",
		Define: func(d *Def) error {
			d.Driver("main", "noop", WithURL("noop://"))
			d.Step("setup", okHandler{}).OnErr(ModeSilent)
			d.Step("run", okHandler{}).
				Closed(4, 3*time.Second).
				After("setup").
				If(func(*Run) bool { return true }).
				Uses("main")
			d.Variant("full")
			return nil
		},
	}
	// Mirror runMain's probe path: bags -> paramSet -> standard params ->
	// Define -> schema -> probe.
	set := newParamSet(nil, envMap(nil), nil)
	seedP, drvURL, drvKind, _, variantP := registerStandardParams(tst, set)
	registerRetryParams(tst, set)
	rootSeed, err := resolveSeed(seedP.Value(), tst.Seed)
	if err != nil {
		t.Fatalf("resolveSeed: %v", err)
	}
	run := &Run{test: tst, seed: rootSeed, getenv: envMap(nil), stdDriverURL: drvURL, stdDriverKind: drvKind}
	d := newDef(tst, set, run)
	if err := tst.Define(d); err != nil {
		t.Fatalf("Define: %v", err)
	}
	if len(d.drivers) > 0 {
		patchDriverDefault(drvURL, d.drivers[0].spec.URL)
		patchDriverDefault(drvKind, d.drivers[0].kind)
	}
	if err := set.Err(); err != nil {
		t.Fatalf("param set: %v", err)
	}
	active, activeSet, err := selectVariantSteps(d, variantP.Value())
	if err != nil {
		t.Fatalf("selectVariantSteps: %v", err)
	}
	pruneEdges(active, activeSet)
	var sb strings.Builder
	if err := writeProbe(&sb, buildProbe(tst, active, rootSeed, set.Schema(), run.slots, run, d, variantP.Value())); err != nil {
		t.Fatalf("writeProbe: %v", err)
	}
	got := sb.String()
	want := `{
  "name": "golden",
  "seed": 42,
  "variant": "full",
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
    },
    {
      "name": "insert.method",
      "env": "STROPPY_INSERT_METHOD",
      "flag": "--insert.method",
      "config": "insert.method",
      "type": "string",
      "help": "slot-0 default insert method (native|plain_query|plain_bulk|columnar)",
      "default": "native",
      "current": "native",
      "source": "default"
    },
    {
      "name": "variant",
      "env": "VARIANT",
      "flag": "--variant",
      "config": "variant",
      "type": "string",
      "help": "active variant subgraph (declared by the test)",
      "default": "full",
      "current": "full",
      "source": "default"
    },
    {
      "name": "retry.attempts",
      "env": "STROPPY_RETRY_ATTEMPTS",
      "flag": "--retry.attempts",
      "config": "retry.attempts",
      "type": "int",
      "help": "total attempts per retried call (1 = disabled)",
      "default": "0",
      "current": "0",
      "source": "default"
    },
    {
      "name": "retry.backoff",
      "env": "STROPPY_RETRY_BACKOFF",
      "flag": "--retry.backoff",
      "config": "retry.backoff",
      "type": "duration",
      "help": "sleep between retries (0 = none)",
      "default": "0s",
      "current": "0s",
      "source": "default"
    },
    {
      "name": "retry.jitter",
      "env": "STROPPY_RETRY_JITTER",
      "flag": "--retry.jitter",
      "config": "retry.jitter",
      "type": "duration",
      "help": "uniform jitter added to each backoff (0 = none)",
      "default": "0s",
      "current": "0s",
      "source": "default"
    }
  ],
  "drivers": [
    {
      "name": "main",
      "kind": "noop",
      "url": "noop://",
      "mode": "per-vu"
    }
  ],
  "variants": [
    "full"
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
