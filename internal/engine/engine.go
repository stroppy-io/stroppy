// Package engine is the k6-free native runtime: a sobek VM that runs the
// existing TypeScript workloads unchanged, with the stroppy module, executors,
// and metrics owned by stroppy instead of k6. Selected via STROPPY_OWN_ENGINE=1.
package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	js "github.com/grafana/sobek"
	"github.com/grafana/sobek/parser"
	k6metrics "go.k6.io/k6/metrics"
	"go.uber.org/zap"

	_ "embed"
)

// TranspileTypeScript transpiles a TS entry to a single JS bundle, with k6 and
// the stroppy parse_sql helper left external (resolved as VM globals).
func TranspileTypeScript(entryPath string) (string, error) {
	entryAbs, err := filepath.Abs(entryPath)
	if err != nil {
		return "", fmt.Errorf("absolute path: %w", err)
	}
	dirAbs := filepath.Dir(entryAbs)

	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{entryAbs},
		Bundle:            true,
		Platform:          api.PlatformNeutral,
		Format:            api.FormatDefault,
		Target:            api.ES2019,
		Sourcemap:         api.SourceMapInline,
		Write:             false,
		LogLevel:          api.LogLevelError,
		AbsWorkingDir:     dirAbs,
		External:          []string{"k6", "k6/x/*", "k6/*", "./parse_sql.js"},
		MainFields:        []string{"module", "main"},
		ResolveExtensions: []string{extTS, extTSX, extJS, extMJS, extJSON},
		Loader: map[string]api.Loader{
			extTS:   api.LoaderTS,
			extTSX:  api.LoaderTSX,
			extJS:   api.LoaderJS,
			extMJS:  api.LoaderJS,
			extJSON: api.LoaderJSON,
		},
	})
	if len(result.Errors) > 0 {
		return "", fmt.Errorf("esbuild: %s", result.Errors[0].Text)
	}
	if len(result.OutputFiles) == 0 {
		return "", errors.New("esbuild: no output")
	}
	return string(result.OutputFiles[0].Contents), nil
}

// reEncodingObjectImport rewrites `import encoding from "k6/x/encoding"` to a
// const backed by the polyfilled globals (the VM has no k6/x/encoding module).
var reEncodingObjectImport = regexp.MustCompile(`import\s+(\w+)\s+from\s+["']k6/x/encoding["'];?`)

//go:embed encodersPolyfill.js
var encodersPolyfill string

// Run executes a single transpiled workload script on the native engine.
// envs is the full KEY=VALUE environment (os.Environ + stroppy overrides);
// tempDir is the staged workdir (script + SQL + parse_sql.js live here).
func Run(ctx context.Context, scriptPath, tempDir string, envs []string, lg *zap.Logger) error {
	root = newRootState(lg, ctx)
	defer root.Teardown()

	envMap := envSliceToMap(envs)

	dirBefore, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getcwd: %w", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		return fmt.Errorf("chdir tempdir: %w", err)
	}
	defer os.Chdir(dirBefore) //nolint:errcheck

	jsCode, err := TranspileTypeScript(scriptPath)
	if err != nil {
		return fmt.Errorf("transpile: %w", err)
	}
	jsCode = reEncodingObjectImport.ReplaceAllString(jsCode,
		`const $1 = { TextEncoder: globalThis.TextEncoder, TextDecoder: globalThis.TextDecoder };`)

	sum := newSummary(root)
	sum.start(ctx)
	defer sum.print()

	// Init pass: one runtime runs module scope + setup() + (later) teardown().
	// Drivers declared at module scope are shared (initPhase).
	initVU := newVU(root, 1, ctx)
	initVU.initPhase = true
	initVM, err := setupRuntime(initVU, envMap, jsCode)
	if err != nil {
		return fmt.Errorf("module scope: %w", err)
	}

	scenarios, err := readScenarios(initVM)
	if err != nil {
		return fmt.Errorf("options: %w", err)
	}

	initVU.initPhase = false
	initVU.ctx = ctx
	if err := execJSFunc(initVM, "setup", false); err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	// Scenarios: each runs its exec fn across VUs on fresh per-VU runtimes.
	for _, sc := range scenarios {
		if err := runScenario(ctx, root, envMap, jsCode, sc, lg); err != nil {
			return fmt.Errorf("scenario %q: %w", sc.name, err)
		}
	}

	if err := execJSFunc(initVM, "teardown", false); err != nil {
		return fmt.Errorf("teardown: %w", err)
	}
	return nil
}

func newVU(r *RootState, vuid uint64, ctx context.Context) *VU {
	return &VU{root: r, rt: js.New(), vuid: vuid, ctx: ctx}
}

// setupRuntime creates the sobek VM for a VU, installs all globals (k6 subset +
// stroppy module exports + real parse_sql), runs the bundle's module scope.
// Returns the runtime with exported functions (options/setup/workload/...) bound.
func setupRuntime(vu *VU, envMap map[string]string, jsCode string) (*js.Runtime, error) {
	vm := vu.rt
	vm.SetParserOptions(parser.IsModule)
	vm.SetFieldNameMapper(js.UncapFieldNameMapper())

	if _, err := vm.RunString(encodersPolyfill); err != nil {
		return nil, fmt.Errorf("encoder polyfill: %w", err)
	}

	if err := installK6Globals(vm, vu, envMap); err != nil {
		return nil, err
	}
	if err := installStroppyGlobals(vm, vu); err != nil {
		return nil, err
	}
	if err := installParseSQL(vm); err != nil {
		return nil, err
	}

	if _, err := vm.RunString(jsCode); err != nil {
		return nil, fmt.Errorf("run bundle: %w", err)
	}
	return vm, nil
}

func installK6Globals(vm *js.Runtime, vu *VU, envMap map[string]string) error {
	envObj := vm.NewObject()
	for k, v := range envMap {
		_ = envObj.Set(k, v)
	}

	console := vm.NewObject()
	consoleOut := func(level string) func(js.FunctionCall) js.Value {
		return func(call js.FunctionCall) js.Value {
			args := make([]any, 0, len(call.Arguments))
			for _, a := range call.Arguments {
				args = append(args, a.String())
			}
			vu.root.lg.Named("console").Info(fmt.Sprint(args...))
			return js.Undefined()
		}
	}
	_ = console.Set("log", consoleOut("log"))
	_ = console.Set("warn", consoleOut("warn"))
	_ = console.Set("error", consoleOut("error"))

	exec := vm.NewObject()
	test := vm.NewObject()
	execVu := vm.NewObject()
	abortFn := func(msg string) { panic(abortSentinel{msg}) }
	_ = test.Set("abort", func(call js.FunctionCall) js.Value { abortFn(fmt.Sprint(call.Argument(0))); return js.Undefined() })
	_ = test.Set("fail", func(call js.FunctionCall) js.Value { panic(fmt.Errorf("test fail: %v", call.Argument(0))) })
	_ = execVu.Set("idInTest", vu.vuid)
	_ = exec.Set("test", test)
	_ = exec.Set("vu", execVu)

	mocks := Mocks{
		{"__ENV", envObj},
		{"__VU", vu.vuid},
		{"open", func(p string) string {
			b, err := os.ReadFile(p)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("open %q: %w", p, err)))
			}
			return string(b)
		}},
		{"console", console},
		{"sleep", func(t float64) { time.Sleep(time.Duration(t * float64(time.Second))) }},
		{"sleepMs", func(ms float64) { time.Sleep(time.Duration(ms) * time.Millisecond) }},
		{"exec", exec},
		{"test", test},
		{"Counter", vu.metricCtor(k6metrics.Counter)},
		{"Rate", vu.metricCtor(k6metrics.Rate)},
		{"Trend", vu.metricCtor(k6metrics.Trend)},
	}
	return mocks.Set(vm)
}

// installStroppyGlobals exposes the engine module's named exports as globals
// (sobek binds `import {X} from "k6/x/stroppy"` to a global named X).
func installStroppyGlobals(vm *js.Runtime, vu *VU) error {
	inst := NewInstance(vu)
	for name, value := range inst.Exports() {
		if err := vm.Set(name, value); err != nil {
			return fmt.Errorf("set global %q: %w", name, err)
		}
	}
	return nil
}

// reParseSQLExport matches the trailing `export{<a> as parse_sql, <b> as parse_sql_with_sections};`
// and rewrites it to assign the two functions onto globalThis, so the workload's
// `import {...} from "./parse_sql.js"` binds to the real implementation. JS
// identifiers may include `$` (esbuild minifies to names like $C), hence the
// custom ident class instead of \w.
var jsIdent = `[$A-Za-z_][$A-Za-z0-9_]*`

var reParseSQLExport = regexp.MustCompile(
	`export\s*\{\s*(` + jsIdent + `)\s+as\s+parse_sql\s*,\s*(` + jsIdent + `)\s+as\s+parse_sql_with_sections\s*\}\s*;?`,
)

// installParseSQL loads the bundled parse_sql.js, rewrites its ESM export to
// globalThis assignments, and evaluates it so parse_sql/parse_sql_with_sections
// are available as globals.
func installParseSQL(vm *js.Runtime) error {
	src, err := os.ReadFile("parse_sql.js")
	if err != nil {
		// Workloads that don't parse SQL don't ship the file; nothing to do.
		return nil
	}
	code := reParseSQLExport.ReplaceAllString(string(src),
		`globalThis.parse_sql = $1; globalThis.parse_sql_with_sections = $2;`)
	if _, err := vm.RunString(code); err != nil {
		return fmt.Errorf("parse_sql.js: %w", err)
	}
	return nil
}

type abortSentinel struct{ msg string }

// --- metrics constructors (Counter/Rate/Trend) ---

// customMetric is the JS-visible object returned by new Counter/Rate/Trend.
// .add(value, tags?) pushes a k6 Sample into the engine sink.
type customMetric struct {
	root *RootState
	m    *k6metrics.Metric
}

func (vu *VU) metricCtor(typ k6metrics.MetricType) func(call js.ConstructorCall) *js.Object {
	return func(call js.ConstructorCall) *js.Object {
		name := call.Argument(0).String()
		m, err := vu.root.registry.NewMetric(name, typ)
		if err != nil {
			panic(vu.rt.NewTypeError("can't register metric %q: %v", name, err))
		}
		cm := &customMetric{root: vu.root, m: m}
		obj := call.This
		_ = obj.Set("add", cm.Add)
		return obj
	}
}

func (c *customMetric) Add(call js.FunctionCall) js.Value {
	v := call.Argument(0).ToFloat()
	tags := c.root.registry.RootTagSet()
	k6metrics.PushIfNotDone(context.Background(), c.root.samples, k6metrics.Sample{
		TimeSeries: k6metrics.TimeSeries{Metric: c.m, Tags: tags},
		Time:       time.Now(),
		Value:      v,
	})
	return js.Undefined()
}

// --- scenarios (read straight from the exported `options` object) ---

type scenarioSpec struct {
	name       string
	executor   string
	exec       string
	vus        int
	iterations int64
	duration   time.Duration
}

func readScenarios(vm *js.Runtime) ([]scenarioSpec, error) {
	opts := vm.Get("options")
	if opts == nil || js.IsUndefined(opts) || js.IsNull(opts) {
		return nil, errors.New("script exports no `options`")
	}
	scenariosVal := opts.ToObject(vm).Get("scenarios")
	if scenariosVal == nil || js.IsUndefined(scenariosVal) || js.IsNull(scenariosVal) {
		return nil, errors.New("`options` has no scenarios")
	}

	var out []scenarioSpec
	for _, name := range scenariosVal.ToObject(vm).Keys() {
		sc := scenariosVal.ToObject(vm).Get(name).ToObject(vm)
		spec := scenarioSpec{
			name:     name,
			executor: getString(sc, "executor"),
			exec:     getString(sc, "exec"),
			vus:      int(getInteger(sc, "vus")),
		}
		if spec.exec == "" {
			spec.exec = "default" // k6 default: the `export default function`
		}
		if spec.vus < 1 {
			spec.vus = 1
		}
		spec.iterations = getInteger(sc, "iterations")
		if d := getString(sc, "duration"); d != "" {
			dur, err := time.ParseDuration(d)
			if err != nil {
				return nil, fmt.Errorf("scenario %q duration %q: %w", name, d, err)
			}
			spec.duration = dur
		}
		out = append(out, spec)
	}
	return out, nil
}

func getString(o *js.Object, k string) string {
	v := o.Get(k)
	if v == nil || js.IsUndefined(v) || js.IsNull(v) {
		return ""
	}
	return v.String()
}

func getInteger(o *js.Object, k string) int64 {
	v := o.Get(k)
	if v == nil || js.IsUndefined(v) || js.IsNull(v) {
		return 0
	}
	return v.ToInteger()
}

func execJSFunc(vm *js.Runtime, funcName string, required bool) error {
	if funcName == "default" {
		names := vm.GlobalObject().GetOwnPropertyNames()
		if idx := indexOfDefault(names); idx != -1 {
			funcName = names[idx]
		}
	}
	fnValue := vm.Get(funcName)
	if fnValue == nil || js.IsUndefined(fnValue) {
		if required {
			return fmt.Errorf("function %q not defined", funcName)
		}
		return nil
	}
	fn, ok := js.AssertFunction(fnValue)
	if !ok {
		if required {
			return fmt.Errorf("%q is not a function", funcName)
		}
		return nil
	}
	if _, err := fn(js.Undefined()); err != nil {
		return unwrapJSError(err)
	}
	return nil
}

func unwrapJSError(err error) error {
	var jsEx *js.Exception
	if errors.As(err, &jsEx) {
		if _, ok := jsEx.Value().Export().(abortSentinel); ok {
			return errAbort
		}
	}
	if errors.Is(err, errAbort) {
		return err
	}
	return err
}

var errAbort = errors.New("aborted")

func indexOfDefault(names []string) int {
	for i, n := range names {
		if contains(n, "default") {
			return i
		}
	}
	return -1
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// --- executors ---

func runScenario(ctx context.Context, r *RootState, envMap map[string]string, jsCode string, sc scenarioSpec, lg *zap.Logger) error {
	vus := sc.vus
	execFn := sc.exec

	switch sc.executor {
	case "shared-iterations":
		var remaining int64 = sc.iterations
		var wg sync.WaitGroup
		for i := 0; i < vus; i++ {
			wg.Add(1)
			go func(vuid int) {
				defer wg.Done()
				runWorkerLoop(ctx, r, envMap, jsCode, vuid, execFn, func() bool {
					return atomic.AddInt64(&remaining, -1) >= 0
				})
			}(i + 1)
		}
		wg.Wait()
	case "constant-vus":
		deadline := time.Now().Add(sc.duration)
		var wg sync.WaitGroup
		for i := 0; i < vus; i++ {
			wg.Add(1)
			go func(vuid int) {
				defer wg.Done()
				runWorkerLoop(ctx, r, envMap, jsCode, vuid, execFn, func() bool {
					return time.Now().Before(deadline)
				})
			}(i + 1)
		}
		wg.Wait()
	default:
		return fmt.Errorf("executor type %q not supported by engine floor", sc.executor)
	}
	return nil
}

// runWorkerLoop builds a fresh per-VU runtime, runs module scope, then drives
// the exec fn while keep() is true. Errors abort the worker (returned via panic
// captured into errCh by caller path); for the floor we log and stop.
func runWorkerLoop(ctx context.Context, r *RootState, envMap map[string]string, jsCode string, vuid int, execFn string, keep func() bool) {
	vu := newVU(r, uint64(vuid), ctx)
	vu.initPhase = true
	vm, err := setupRuntime(vu, envMap, jsCode)
	if err != nil {
		r.lg.Error("worker init failed", zap.Int("vu", vuid), zap.Error(err))
		return
	}
	vu.initPhase = false
	for keep() {
		vu.ctx = ctx
		if err := execJSFunc(vm, execFn, true); err != nil {
			if errors.Is(err, errAbort) {
				return
			}
			r.lg.Error("iteration failed", zap.Int("vu", vuid), zap.Error(err))
			return
		}
	}
}

// --- summary (drain samples, print) ---

type metricSink struct {
	typ   k6metrics.MetricType
	count uint64
	sum   float64
	vals  []float64 // for trend percentiles
}

type summary struct {
	root  *RootState
	mu    sync.Mutex
	sinks map[string]*metricSink
}

func newSummary(r *RootState) *summary {
	return &summary{root: r, sinks: make(map[string]*metricSink)}
}

func (s *summary) start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sc, ok := <-s.root.samples:
				if !ok {
					return
				}
				s.ingest(sc)
			}
		}
	}()
}

func (s *summary) ingest(sc k6metrics.SampleContainer) {
	for _, sample := range sc.GetSamples() {
		name := sample.Metric.Name
		s.mu.Lock()
		sk, ok := s.sinks[name]
		if !ok {
			sk = &metricSink{typ: sample.Metric.Type}
			s.sinks[name] = sk
		}
		sk.count++
		sk.sum += sample.Value
		if sk.typ == k6metrics.Trend {
			sk.vals = append(sk.vals, sample.Value)
		}
		s.mu.Unlock()
	}
}

func (s *summary) print() {
	close(s.root.samples)
	// drain remainder
	for sc := range s.root.samples {
		s.ingest(sc)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sinks) == 0 {
		fmt.Fprintln(os.Stderr, "engine: no metrics recorded")
		return
	}
	names := make([]string, 0, len(s.sinks))
	for n := range s.sinks {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintln(os.Stderr, "\n=== engine summary ===")
	for _, n := range names {
		sk := s.sinks[n]
		switch sk.typ {
		case k6metrics.Counter:
			fmt.Fprintf(os.Stderr, "  %-40s %d\n", n, uint64(sk.sum))
		case k6metrics.Trend:
			fmt.Fprintf(os.Stderr, "  %-40s count=%d avg=%.3f %s\n", n, sk.count, sk.sum/float64(max1(sk.count)), percentiles(sk.vals))
		case k6metrics.Rate:
			pct := 0.0
			if sk.count > 0 {
				pct = 100 * sk.sum / float64(sk.count)
			}
			fmt.Fprintf(os.Stderr, "  %-40s %.2f%%  %d out of %d\n", n, pct, uint64(sk.sum), sk.count)
		default:
			fmt.Fprintf(os.Stderr, "  %-40s count=%d sum=%.3f\n", n, sk.count, sk.sum)
		}
	}
}

func max1(n uint64) uint64 {
	if n == 0 {
		return 1
	}
	return n
}

func percentiles(vals []float64) string {
	if len(vals) == 0 {
		return ""
	}
	s := make([]float64, len(vals))
	copy(s, vals)
	sort.Float64s(s)
	pct := func(p float64) float64 { return s[int(float64(len(s)-1)*p)] }
	return fmt.Sprintf("p(50)=%.3f p(90)=%.3f p(95)=%.3f p(99)=%.3f", pct(0.5), pct(0.9), pct(0.95), pct(0.99))
}

func envSliceToMap(envs []string) map[string]string {
	m := make(map[string]string, len(envs))
	for _, e := range envs {
		if k, v, ok := splitEnv(e); ok {
			m[k] = v
		}
	}
	return m
}

func splitEnv(e string) (string, string, bool) {
	for i := 0; i < len(e); i++ {
		if e[i] == '=' {
			return e[:i], e[i+1:], true
		}
	}
	return "", "", false
}
