package bench

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/stroppy-io/stroppy/next/dag"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/metrics"
)

// Main parses flags, environment and config, builds and runs the test's step
// DAG, prints per-step statuses and a metrics summary, and exits: 0 on success,
// non-zero on any step failure or configuration error. It is the entry point of
// a test's `func main`.
//
// Params are defined once (struct tags on Test.Opts, plus SDK-injected standard
// params) and projected uniformly to --flags, env vars, and a flat JSON config
// file, with precedence cli > env > config > default (D1). SDK control flags
// (-steps/-no-steps/-probe/-plan/-plan-dot/-cpuprofile/--config/-help) are not
// params and are parsed alongside them. -help lists STANDARD then TEST params.
func Main(t *Test) {
	os.Exit(runMain(t, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// runMain is the testable core of [Main]: it takes the argument list, an env
// lookup and the output streams explicitly and returns the process exit code
// instead of calling os.Exit.
func runMain(t *Test, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	var f sdkFlags
	cli := make(map[string]string)
	if _, err := parseFlags(args, &f, cli); err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintf(stderr, "  run with -help for the option list.\n")
		return 2
	}

	// Phase 1: build the input bags BEFORE declaring any param, so each handle
	// resolves its value at registration (immediate-mode). The config file is a
	// flat, registry-keyed JSON document (no mapping layer).
	cfg, cfgErr := loadConfig(f.config)
	if cfgErr != nil && !f.help {
		fmt.Fprintln(stderr, cfgErr)
		return 1
	}

	set := newParamSet(cli, getenv, cfg)
	seedParam, drvURL, drvKind := registerStandardParams(t, set)
	optsErr := parseOptions(t.Opts, set)

	// -help short-circuits after the registry is populated (params + their
	// resolved defaults are all known), so a config error or an unknown -e does
	// not block the operator from discovering the surface.
	if f.help {
		writeHelp(stdout, t, set)
		return 0
	}
	if optsErr != nil {
		fmt.Fprintln(stderr, optsErr)
		return 1
	}
	if err := set.Err(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := set.checkUnknown(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := validateOptions(t.Opts); err != nil {
		fmt.Fprintf(stderr, "bench: invalid options: %v\n", err)
		return 1
	}

	rootSeed, err := resolveSeed(seedParam.Value(), t.Seed)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	slots := resolveSlots(t.Drivers, drvURL.Value(), drvKind.Value(), getenv)
	run := &Run{test: t, seed: rootSeed, slots: slots, getenv: getenv}
	steps := buildSteps(t, run)

	if f.probe {
		if err := writeProbe(stdout, buildProbe(t, steps, rootSeed, set.Schema(), slots, run)); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if f.steps != "" && f.noSteps != "" {
		fmt.Fprintln(stderr, "bench: -steps and -no-steps are mutually exclusive")
		return 2
	}
	filter := buildStepFilter(f.steps, f.noSteps)

	drivers, err := buildDrivers(slots)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	// Report the effective seed before the run starts: for --seed=auto the
	// operator needs to see the chosen number to reproduce the dataset.
	fmt.Fprintf(stderr, "%s: seed=%d (source: %s)\n", t.Name, rootSeed, seedParam.Source())

	reg := metrics.NewRegistry()
	built, execs, err := buildGraph(steps, run, rootSeed, reg, drivers, slots, filter)
	if err != nil {
		fmt.Fprintf(stderr, "bench: %v\n", err)
		return 1
	}

	if f.plan {
		fmt.Fprint(stdout, dag.PlanText(built))
		return 0
	}
	if f.planDOT {
		fmt.Fprint(stdout, dag.PlanDOT(built))
		return 0
	}

	if f.cpuProf != "" {
		stop, err := startCPUProfile(f.cpuProf)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		defer stop()
	}

	return execute(built, execs, drivers, reg, stdout, steps)
}

// sdkFlags carries the parsed SDK control flags (everything that is not a param).
type sdkFlags struct {
	steps   string
	noSteps string
	probe   bool
	plan    bool
	planDOT bool
	cpuProf string
	config  string
	help    bool
}

// sdkBoolFlags / sdkValFlags partition the SDK control set by shape. Everything
// else on the command line is a param flag (--name=val) collected into cli.
var (
	sdkBoolFlags = map[string]bool{"probe": true, "plan": true, "plan-dot": true, "help": true, "h": true}
	sdkValFlags  = map[string]bool{"steps": true, "no-steps": true, "cpuprofile": true, "config": true}
)

// parseFlags splits args into SDK control flags (written into f) and param flags
// (collected into cli as name->value). A param flag must use the --name=value
// form so the phase order stays "collect bags, then register params" (a stdlib
// FlagSet per entry cannot register before Parse and would invert that). SDK
// control flags accept both -flag value and -flag=value. A bare "--" marks the
// rest as passthrough (returned, currently unused). Both - and -- prefixes are
// accepted for either kind; classification is by name, not dash count.
func parseFlags(args []string, f *sdkFlags, cli map[string]string) (passthrough []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return args[i+1:], nil
		}
		if len(a) < 2 || a[0] != '-' {
			return nil, fmt.Errorf("unexpected argument %q", a)
		}
		body := strings.TrimLeft(a, "-")
		name := body
		val := ""
		hasEq := false
		if idx := strings.IndexByte(body, '='); idx >= 0 {
			name, val, hasEq = body[:idx], body[idx+1:], true
		}
		switch {
		case sdkValFlags[name]:
			if !hasEq {
				i++
				if i >= len(args) {
					return nil, fmt.Errorf("-%s needs a value", name)
				}
				val = args[i]
			}
			switch name {
			case "steps":
				f.steps = val
			case "no-steps":
				f.noSteps = val
			case "cpuprofile":
				f.cpuProf = val
			case "config":
				f.config = val
			}
		case sdkBoolFlags[name]:
			if hasEq {
				b, e := strconv.ParseBool(val)
				if e != nil {
					return nil, fmt.Errorf("-%s=%s: %v", name, val, e)
				}
				setSDKBool(f, name, b)
			} else {
				setSDKBool(f, name, true)
			}
		case name == "":
			return nil, fmt.Errorf("bare dash argument %q", a)
		default:
			// Param flag: require the =value form.
			if !hasEq {
				return nil, fmt.Errorf("flag --%s needs a value (use --%s=value)", name, name)
			}
			cli[name] = val
		}
	}
	return nil, nil
}

// setSDKBool writes a parsed bool SDK control flag into f.
func setSDKBool(f *sdkFlags, name string, b bool) {
	switch name {
	case "probe":
		f.probe = b
	case "plan":
		f.plan = b
	case "plan-dot":
		f.planDOT = b
	case "help", "h":
		f.help = b
	}
}

// loadConfig reads a flat, registry-keyed JSON config file into a
// map[string]json.RawMessage. Keys are param names; values carry their type in
// JSON (a string "5s" for a duration, a number 4 for an int). encoding/json
// only; there is no struct mapping layer.
func loadConfig(path string) (map[string]json.RawMessage, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bench: read config %q: %w", path, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("bench: parse config %q: %w", path, err)
	}
	return m, nil
}

// registerStandardParams declares the SDK-injected STANDARD params every test
// shares: the run seed (string, F6) and the slot-0 driver url/kind. The seed's
// declared default is Test.Seed (the spec-representative value), or "0" when the
// test leaves it unset (D11: 0 is a valid seed).
func registerStandardParams(t *Test, set *paramSet) (seed, drvURL, drvKind *param[string]) {
	seedDef := t.Seed
	if seedDef == "" {
		seedDef = "0"
	}
	seed = set.String("seed", seedDef,
		"run root seed: auto|now (random per run), fixed|canonical (this test's spec seed), or a uint64",
		optEnv("SEED"), optStandard())

	drvURLDef, drvKindDef := "", "pg"
	if len(t.Drivers) > 0 {
		drvURLDef = t.Drivers[0].URL
		drvKindDef = t.Drivers[0].Kind
		if drvKindDef == "" {
			drvKindDef = "pg"
		}
	}
	drvURL = set.String("driver.url", drvURLDef, "slot-0 database URL",
		optEnv("STROPPY_DRIVER_URL"), optStandard())
	drvKind = set.String("driver.kind", drvKindDef, "slot-0 driver kind (pg|noop)",
		optEnv("STROPPY_DRIVER_KIND"), optStandard())
	return seed, drvURL, drvKind
}

// resolveSeed turns the seed param's string value into the uint64 root fed to
// DeriveStream. Keywords (F6): "auto"/"now" draw a fresh random seed per run;
// "fixed"/"canonical" resolve to the test's spec seed (canonicalSeed = Test.Seed,
// or "0" if empty); any other value parses as a uint64 literal.
func resolveSeed(seedVal, canonicalSeed string) (uint64, error) {
	switch strings.ToLower(seedVal) {
	case "auto", "now":
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return 0, fmt.Errorf("bench: seed: %w", err)
		}
		return binary.BigEndian.Uint64(b[:]), nil
	case "fixed", "canonical":
		s := canonicalSeed
		if s == "" {
			s = "0"
		}
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("bench: canonical seed %q: %w", s, err)
		}
		return n, nil
	}
	n, err := strconv.ParseUint(seedVal, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bench: seed %q: %w", seedVal, err)
	}
	return n, nil
}

// writeHelp prints the auto-generated option list, grouped STANDARD (SDK) then
// TEST (author), followed by the SDK control flags. Param defaults and env names
// come from the resolved registry.
func writeHelp(w io.Writer, t *Test, set *paramSet) {
	fmt.Fprintf(w, "Usage: %s [options]\n\n", t.Name)
	fmt.Fprintf(w, "Source precedence for every param: --flag  >  env  >  config file  >  default.\n\n")
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "STANDARD options (SDK-injected):")
	for _, d := range set.order {
		if d.standard {
			writeParamHelp(tw, d)
		}
	}
	if hasTestParams(set) {
		fmt.Fprintln(tw)
		fmt.Fprintln(tw, "TEST options:")
		for _, d := range set.order {
			if !d.standard {
				writeParamHelp(tw, d)
			}
		}
	}
	fmt.Fprintln(tw)
	fmt.Fprintln(tw, "OTHER:")
	fmt.Fprintln(tw, "  --config=<path>\tload params from a flat JSON config file")
	fmt.Fprintln(tw, "  -steps=<list>\tcomma-separated steps to run (exclusive with -no-steps)")
	fmt.Fprintln(tw, "  -no-steps=<list>\tcomma-separated steps to skip")
	fmt.Fprintln(tw, "  -probe\tprint the test description as JSON and exit")
	fmt.Fprintln(tw, "  -plan\tprint the DAG plan (text) and exit")
	fmt.Fprintln(tw, "  -plan-dot\tprint the DAG plan (Graphviz DOT) and exit")
	fmt.Fprintln(tw, "  -cpuprofile=<file>\twrite a pprof CPU profile")
	fmt.Fprintln(tw, "  -help\tshow this help")
	_ = tw.Flush()
}

func writeParamHelp(tw *tabwriter.Writer, d *paramDecl) {
	fmt.Fprintf(tw, "  --%s=<%s>\t%s  [env %s]  (default: %s)\n",
		d.name, d.kind, d.help, d.env, d.defStr)
}

func hasTestParams(set *paramSet) bool {
	for _, d := range set.order {
		if !d.standard {
			return true
		}
	}
	return false
}

// buildSteps invokes the test's step builder (nil-safe: a Test with no Build
// contributes no steps).
func buildSteps(t *Test, run *Run) []*StepDef {
	if t.Build == nil {
		return nil
	}
	return t.Build(run)
}

// startCPUProfile begins writing a pprof CPU profile to path and returns a stop
// function that ends the profile and closes the file. It backs the hidden
// -cpuprofile flag (the M7 pprof pass).
func startCPUProfile(path string) (func(), error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("bench: create cpu profile: %w", err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("bench: start cpu profile: %w", err)
	}
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}, nil
}

// execute materializes every executor, spans them with one run-level reporter,
// runs the DAG, prints per-step statuses, tears the drivers down and returns the
// exit code.
func execute(built *dag.Built, execs []*Executor, drivers []driver.Driver, reg *metrics.Registry, stdout io.Writer, steps []*StepDef) int {
	// Freeze the shared registry and materialize every executor's shards, then
	// span them with a single run-level reporter.
	shards := materializeAll(reg, execs)
	reporter := metrics.NewReporter(reg, shards, metrics.DefaultInterval, metrics.NewConsoleSink(stdout))

	ctx := context.Background()
	reporter.Start()
	result := dag.Run(ctx, built)
	printStatuses(stdout, steps, result)
	reporter.Stop() // writers stopped; emits the exact run summary

	for _, d := range drivers {
		_ = d.Teardown(ctx)
	}

	if result.Status != dag.Succeeded {
		return 1
	}
	return 0
}

// buildGraph builds one executor per step over the shared registry and assembles
// the dag with each step's edges and combined condition, then validates it.
func buildGraph(steps []*StepDef, run *Run, seed uint64, reg *metrics.Registry, drivers []driver.Driver, slots []slotSpec, filter stepFilter) (*dag.Built, []*Executor, error) {
	g := dag.NewGraph()
	execs := make([]*Executor, 0, len(steps))
	for _, sd := range steps {
		slot, err := resolveUses(sd, slots)
		if err != nil {
			return nil, nil, err
		}
		ex := buildExecutor(stepConfig(sd, seed, reg, drivers, slot), sd)
		execs = append(execs, ex)
		g.Add(&dag.Node{
			ID:        sd.name,
			Run:       ex.Run,
			If:        nodeIf(sd, run, filter),
			After:     sd.after,
			AfterAny:  sd.afterAny,
			OnFailure: sd.onFailure,
		})
	}
	built, err := g.Build()
	if err != nil {
		return nil, nil, err
	}
	return built, execs, nil
}

// nodeIf combines the -steps/-no-steps filter with the step's own If predicate
// into the dag node's condition (nil when neither is set). A step runs only when
// the filter admits it and its predicate returns true.
func nodeIf(sd *StepDef, run *Run, filter stepFilter) func() bool {
	if sd.ifPred == nil && filter == nil {
		return nil
	}
	return func() bool {
		if filter != nil && !filter(sd.name) {
			return false
		}
		if sd.ifPred != nil {
			return sd.ifPred(run)
		}
		return true
	}
}

// stepFilter reports whether a step name is admitted by the -steps/-no-steps
// selection.
type stepFilter func(name string) bool

// buildStepFilter turns the -steps / -no-steps values into a filter (nil when
// neither is set). -steps admits only listed names; -no-steps admits all but the
// listed names. The caller guarantees at most one is non-empty.
func buildStepFilter(steps, noSteps string) stepFilter {
	if steps != "" {
		set := toSet(steps)
		return func(n string) bool { return set[n] }
	}
	if noSteps != "" {
		set := toSet(noSteps)
		return func(n string) bool { return !set[n] }
	}
	return nil
}

// toSet splits a comma list into a set, trimming spaces and dropping empties.
func toSet(csv string) map[string]bool {
	set := make(map[string]bool)
	for _, p := range strings.Split(csv, ",") {
		if p = strings.TrimSpace(p); p != "" {
			set[p] = true
		}
	}
	return set
}

// printStatuses writes the per-step status table in declaration order.
func printStatuses(w io.Writer, steps []*StepDef, result *dag.RunResult) {
	_, _ = fmt.Fprintf(w, "\n=== steps (%s) ===\n", result.Status)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "step\tstatus\tattempts\tduration")
	for _, sd := range steps {
		r := result.Node(sd.name)
		if r == nil {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t-\t-\n", sd.name, "Absent")
			continue
		}
		dur := "-"
		if !r.Start.IsZero() {
			dur = r.End.Sub(r.Start).Round(1e5).String()
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", sd.name, r.Status, r.Attempts, dur)
	}
	_ = tw.Flush()
}
