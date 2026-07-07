package bench

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"
	"text/tabwriter"

	"github.com/stroppy-io/stroppy/next/dag"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/metrics"
)

// Main parses flags and environment, builds and runs the test's step DAG, prints
// per-step statuses and a metrics summary, and exits: 0 on success, non-zero on
// any step failure or a configuration error. It is the entry point of a test's
// `func main`. The recognized flags are -seed, -steps, -no-steps, -probe, -plan
// and -plan-dot (see runMain).
func Main(t *Test) {
	os.Exit(runMain(t, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// runMain is the testable core of [Main]: it takes the argument list, an env
// lookup and the output streams explicitly and returns the process exit code
// instead of calling os.Exit.
func runMain(t *Test, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(t.Name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		seedFlag    = fs.Uint64("seed", t.Seed, "override the run seed")
		stepsFlag   = fs.String("steps", "", "comma-separated steps to run (others pruned); mutually exclusive with -no-steps")
		noStepsFlag = fs.String("no-steps", "", "comma-separated steps to skip; mutually exclusive with -steps")
		probeFlag   = fs.Bool("probe", false, "print the test description as JSON and exit")
		planFlag    = fs.Bool("plan", false, "print the DAG plan (text) and exit")
		planDOTFlag = fs.Bool("plan-dot", false, "print the DAG plan (Graphviz DOT) and exit")
		// cpuprofile is a hidden diagnostic (not shown in -help): write a pprof
		// CPU profile of the run to the given file. Used for the M7 pprof pass.
		cpuprofileFlag = fs.String("cpuprofile", "", "")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	schema, err := parseOptions(t.Opts, getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := validateOptions(t.Opts); err != nil {
		_, _ = fmt.Fprintf(stderr, "bench: invalid options: %v\n", err)
		return 1
	}

	seed := *seedFlag
	slots := resolveSlots(t.Drivers, getenv)

	// Build the steps once, now that options are parsed and slots resolved, so
	// the builder can size executor policies from the options (no pre-parse).
	run := &Run{test: t, seed: seed, slots: slots, getenv: getenv}
	steps := buildSteps(t, run)

	if *probeFlag {
		if err := writeProbe(stdout, buildProbe(t, steps, seed, schema, slots, run)); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if *stepsFlag != "" && *noStepsFlag != "" {
		_, _ = fmt.Fprintln(stderr, "bench: -steps and -no-steps are mutually exclusive")
		return 2
	}
	filter := buildStepFilter(*stepsFlag, *noStepsFlag)

	drivers, err := buildDrivers(slots)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	reg := metrics.NewRegistry()
	built, execs, err := buildGraph(steps, run, seed, reg, drivers, slots, filter)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "bench: %v\n", err)
		return 1
	}

	if *planFlag {
		_, _ = io.WriteString(stdout, dag.PlanText(built))
		return 0
	}
	if *planDOTFlag {
		_, _ = io.WriteString(stdout, dag.PlanDOT(built))
		return 0
	}

	if *cpuprofileFlag != "" {
		stop, err := startCPUProfile(*cpuprofileFlag)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		defer stop()
	}

	return execute(built, execs, drivers, reg, stdout, steps)
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
