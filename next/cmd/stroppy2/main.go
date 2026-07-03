// Command stroppy2 is the minimal CLI for the stroppy-next engine. It makes a
// test's Go source feel like a script: `stroppy2 run <target>` builds the test
// against the embedded SDK (cached temp module) and executes it; probe/plan
// inspect it; eject writes a built-in's source out for forking.
//
// Targets are a built-in name (simple, tpcc), a .go file, or a package directory.
// The go toolchain on PATH is used to build (toolchain auto-provisioning is
// post-PoC); GOTOOLCHAIN is honoured.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	next "github.com/stroppy-io/stroppy/next"
	"github.com/stroppy-io/stroppy/next/internal/runner"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches the subcommand and returns the process exit code. It is the
// testable core of main: streams and args are explicit.
func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "run":
		return cmdRun(rest, stdout, stderr)
	case "probe":
		return cmdProbe(rest, stdout, stderr)
	case "plan":
		return cmdPlan(rest, stdout, stderr)
	case "eject":
		return cmdEject(rest, stdout, stderr)
	case "version":
		_, _ = fmt.Fprintf(stdout, "stroppy2 %s (SDK %s)\n", next.Version, next.Version)
		return 0
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "stroppy2: unknown command %q\n", cmd)
		usage(stderr)
		return 2
	}
}

func usage(w *os.File) {
	_, _ = fmt.Fprint(w, `stroppy2 — run stroppy-next tests

Usage:
  stroppy2 run   <target> [-e KEY=VAL ...] [--steps a,b | --no-steps a,b] [-- test-flags]
  stroppy2 probe <target>
  stroppy2 plan  <target> [--dot]
  stroppy2 eject <builtin> [dir]
  stroppy2 version

target: a built-in name (simple, tpcc), a .go file, or a package directory.
`)
}

// resolveSource turns a target token into a Source: a built-in name or a path.
func resolveSource(target string) (runner.Source, error) {
	if runner.IsBuiltin(target) {
		return runner.Builtin(target)
	}
	return runner.FromPath(target)
}

// buildTarget resolves and builds a target, reporting the executable path.
func buildTarget(target string, stderr *os.File) (runner.Result, error) {
	src, err := resolveSource(target)
	if err != nil {
		return runner.Result{}, err
	}
	cache, err := runner.NewCache()
	if err != nil {
		return runner.Result{}, err
	}
	return cache.Build(src)
}

// execTest runs a built test binary with extra env and args, streaming its
// output, and returns its exit code.
func execTest(bin string, env, args []string, stdout, stderr *os.File) int {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		_, _ = fmt.Fprintf(stderr, "stroppy2: %v\n", err)
		return 1
	}
	return 0
}

// cmdRun implements `run <target> [flags] [-- test-flags]`.
func cmdRun(args []string, stdout, stderr *os.File) int {
	p, err := parseRun(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "stroppy2: %v\n", err)
		return 2
	}
	res, err := buildTarget(p.target, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "stroppy2: %v\n", err)
		return 1
	}
	return execTest(res.Bin, p.env, p.testArgs(), stdout, stderr)
}

// cmdProbe implements `probe <target>`: build then dump the JSON description.
func cmdProbe(args []string, stdout, stderr *os.File) int {
	return buildAndExec(args, []string{"-probe"}, stdout, stderr)
}

// cmdPlan implements `plan <target> [--dot]`.
func cmdPlan(args []string, stdout, stderr *os.File) int {
	flag := "-plan"
	var rest []string
	for _, a := range args {
		if a == "--dot" || a == "-dot" {
			flag = "-plan-dot"
			continue
		}
		rest = append(rest, a)
	}
	return buildAndExec(rest, []string{flag}, stdout, stderr)
}

// buildAndExec builds the single target in args and execs it with fixed flags.
func buildAndExec(args, testFlags []string, stdout, stderr *os.File) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(stderr, "stroppy2: expected exactly one target")
		return 2
	}
	res, err := buildTarget(args[0], stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "stroppy2: %v\n", err)
		return 1
	}
	return execTest(res.Bin, nil, testFlags, stdout, stderr)
}

// cmdEject implements `eject <builtin> [dir]`.
func cmdEject(args []string, stdout, stderr *os.File) int {
	if len(args) < 1 || len(args) > 2 {
		_, _ = fmt.Fprintln(stderr, "stroppy2: usage: eject <builtin> [dir]")
		return 2
	}
	name := args[0]
	dir := "./" + name
	if len(args) == 2 {
		dir = args[1]
	}
	files, err := runner.Eject(name, dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "stroppy2: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "ejected %s into %s (%d files)\n", name, dir, len(files))
	for _, f := range files {
		_, _ = fmt.Fprintf(stdout, "  %s\n", f)
	}
	return 0
}

// runParams is a parsed `run` invocation.
type runParams struct {
	target      string
	env         []string // -e KEY=VAL, passed to the test process environment
	steps       string   // --steps
	noSteps     string   // --no-steps
	seed        string   // --seed (empty = unset)
	passthrough []string // args after --, forwarded verbatim
}

// testArgs builds the test binary's argument list from the forwarded flags plus
// the verbatim passthrough. The test binary uses single-dash flag syntax.
func (p runParams) testArgs() []string {
	var a []string
	if p.steps != "" {
		a = append(a, "-steps="+p.steps)
	}
	if p.noSteps != "" {
		a = append(a, "-no-steps="+p.noSteps)
	}
	if p.seed != "" {
		a = append(a, "-seed="+p.seed)
	}
	return append(a, p.passthrough...)
}

// parseRun parses `run` args: the first positional is the target, then -e/--steps/
// --no-steps/--seed flags, and everything after a bare -- is forwarded verbatim.
func parseRun(args []string) (runParams, error) {
	var p runParams
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			p.passthrough = args[i+1:]
			break
		}
		switch {
		case a == "-e" || a == "--env":
			i++
			if i >= len(args) {
				return p, fmt.Errorf("%s needs KEY=VAL", a)
			}
			if !strings.Contains(args[i], "=") {
				return p, fmt.Errorf("invalid -e %q, want KEY=VAL", args[i])
			}
			p.env = append(p.env, args[i])
		case strings.HasPrefix(a, "-e="):
			p.env = append(p.env, strings.TrimPrefix(a, "-e="))
		case a == "--steps" || a == "-steps":
			i++
			if i >= len(args) {
				return p, fmt.Errorf("%s needs a value", a)
			}
			p.steps = args[i]
		case strings.HasPrefix(a, "--steps=") || strings.HasPrefix(a, "-steps="):
			p.steps = a[strings.IndexByte(a, '=')+1:]
		case a == "--no-steps" || a == "-no-steps":
			i++
			if i >= len(args) {
				return p, fmt.Errorf("%s needs a value", a)
			}
			p.noSteps = args[i]
		case strings.HasPrefix(a, "--no-steps=") || strings.HasPrefix(a, "-no-steps="):
			p.noSteps = a[strings.IndexByte(a, '=')+1:]
		case a == "--seed" || a == "-seed":
			i++
			if i >= len(args) {
				return p, fmt.Errorf("%s needs a value", a)
			}
			p.seed = args[i]
		case strings.HasPrefix(a, "--seed=") || strings.HasPrefix(a, "-seed="):
			p.seed = a[strings.IndexByte(a, '=')+1:]
		case strings.HasPrefix(a, "-"):
			return p, fmt.Errorf("unknown flag %q (put test flags after --)", a)
		default:
			if p.target != "" {
				return p, fmt.Errorf("unexpected argument %q", a)
			}
			p.target = a
		}
	}
	if p.target == "" {
		return p, fmt.Errorf("missing target")
	}
	if p.steps != "" && p.noSteps != "" {
		return p, fmt.Errorf("--steps and --no-steps are mutually exclusive")
	}
	return p, nil
}
