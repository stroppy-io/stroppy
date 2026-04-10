package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	js "github.com/grafana/sobek"
	"github.com/grafana/sobek/parser"
	"github.com/sirupsen/logrus"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/lib/executor"
	"google.golang.org/protobuf/proto"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"

	_ "embed"
)

var (
	ErrEsbuild         = errors.New("esbuild error")
	ErrNoEsbuildOutput = errors.New("no output from esbuild")

	ErrJSFuncNotDefined = errors.New("function not defined or not exported")
	ErrNotAJSFunc       = errors.New("found, but is not a function")
	ErrCallJSFunc       = errors.New("failed to call js function")
)

// TranspileTypeScript transpiles TypeScript to JavaScript using esbuild.
// TODO: make and reuse, if possible, code -> code version (without path)
func TranspileTypeScript(entryPath string) (string, error) {
	entryAbs, err := filepath.Abs(entryPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	dirAbs := filepath.Dir(entryAbs)

	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{entryAbs},
		Bundle:            true,
		Platform:          api.PlatformNeutral,
		Format:            api.FormatDefault,
		Target:            api.ES2019,
		Sourcemap:         api.SourceMapInline,
		Write:             false, // keep outputs in-memory
		LogLevel:          api.LogLevelError,
		AbsWorkingDir:     dirAbs,
		External:          []string{"k6", "k6/x/*", "k6/*", "./parse_sql.js"},
		MainFields:        []string{"module", "main"},
		ResolveExtensions: []string{".ts", ".tsx", ".js", ".mjs", ".json"},
		Loader: map[string]api.Loader{
			".ts":   api.LoaderTS,
			".tsx":  api.LoaderTSX,
			".js":   api.LoaderJS,
			".mjs":  api.LoaderJS,
			".json": api.LoaderJSON,
		},
	})

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("%w: %s", ErrEsbuild, result.Errors[0].Text)
	}

	if len(result.OutputFiles) == 0 {
		return "", ErrNoEsbuildOutput
	}

	return string(result.OutputFiles[0].Contents), nil
}

// ProbeScript runs the script at the scriptPath in a mocked k6+stroppy runtime.
// Required to probe in a full stroppy workdir with all the scripts.
//
// TODO: Drop the workdir requirement.
// Refactor the transpilation to put workdir scripts at the transpilation phase directly to a user script.
func ProbeScript(scriptPath string) (*Probeprint, error) {
	jsCode, err := TranspileTypeScript(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to transpile TypeScript: %w", err)
	}

	vm := createVM()

	probeprint, err := ProbeJSTest(vm, jsCode)
	if err != nil {
		return nil, err // TODO: wrap
	}

	return probeprint, nil
}

var reEncodingObjectImport = regexp.MustCompile(`import\s+(\w+)\s+from\s+["']k6/x/encoding["'];?`)

// reK6BareImport matches `import { sleep, ... } from "k6"` — the bare "k6"
// module (as opposed to "k6/options", "k6/metrics" etc. which are handled by
// esbuild's k6/* external pattern). The probe VM doesn't resolve modules, so
// we replace the import with a no-op stub object.
var reK6BareImport = regexp.MustCompile(`import\s*\{([^}]+)\}\s*from\s*["']k6["'];?`)

func ProbeJSTest(vm *js.Runtime, jsCode string) (*Probeprint, error) {
	// Mock k6/x/encoding import
	// This is needed because the extraction VM doesn't have the k6/x/encoding module.
	// We replace the import with a const that exposes the polyfilled TextEncoder/TextDecoder.
	jsCode = reEncodingObjectImport.ReplaceAllString(
		jsCode,
		`const $1 = { TextEncoder: globalThis.TextEncoder, TextDecoder: globalThis.TextDecoder };`,
	)

	// Mock bare "k6" import (sleep, check, etc.) — stub all names as no-ops.
	jsCode = reK6BareImport.ReplaceAllStringFunc(jsCode, func(match string) string {
		sub := reK6BareImport.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		var names []string
		for _, n := range strings.Split(sub[1], ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
		// var _k6 = {sleep:function(){}, ...}; var sleep = _k6.sleep; ...
		var stubs, assigns []string
		for _, n := range names {
			stubs = append(stubs, n+":function(){}")
			assigns = append(assigns, "var "+n+" = _k6."+n+";")
		}
		return "var _k6 = {" + strings.Join(stubs, ",") + "}; " + strings.Join(assigns, " ")
	})

	probeprint := &Probeprint{}
	if err := prepareVMEnvironment(vm, probeprint); err != nil {
		return nil, fmt.Errorf("failed to prepare VM environment: %w", err)
	}

	if _, err := vm.RunString(jsCode); err != nil {
		return nil, fmt.Errorf("failed to probe script: %w", err)
	}

	if err := runK6Handles(vm); err != nil {
		return nil, fmt.Errorf("failed to run k6 functions: %w", err)
	}

	options, err := unwrapOptions(vm.Get("options"))
	if err != nil {
		return nil, fmt.Errorf("failed to get options: %w", err)
	}

	probeprint.Options = &options

	return probeprint, nil
}

func execJSFunc(vm *js.Runtime, funcName string, required bool) error {
	// esbuild produces default function with name <test_file_name>_default.
	if funcName == "default" {
		names := vm.GlobalObject().GetOwnPropertyNames()

		idx := slices.IndexFunc(names,
			func(name string) bool { return strings.Contains(name, "default") })
		if idx != -1 {
			funcName = names[idx]
		}
	}

	//nolint: nestif // un-nested is uglier
	if fnValue := vm.Get(funcName); fnValue != nil { // defined
		if fn, ok := js.AssertFunction(fnValue); ok {
			if _, err := fn(js.Undefined()); err != nil { // we need just exec it
				return fmt.Errorf(`%w: '%s()': %w`, ErrCallJSFunc, funcName, err)
			}
		} else if required {
			return fmt.Errorf(`'%s()' %w`, funcName, ErrNotAJSFunc)
		}
	} else if required {
		return fmt.Errorf(`'%s()' %w`, funcName, ErrJSFuncNotDefined)
	}

	return nil
}

func runK6Handles(vm *js.Runtime) error {
	if err := execJSFunc(vm, "setup", false); err != nil {
		return err
	}

	options, err := unwrapOptions(vm.Get("options"))
	if err != nil {
		return err
	}

	scenarios := options.Scenarios.GetSortedConfigs()

	executed := map[string]bool{}
	for _, s := range scenarios {
		if execFuncName := s.GetExec(); !executed[execFuncName] {
			if err := execJSFunc(vm, execFuncName, true); err != nil {
				return err
			}

			executed[execFuncName] = true
		}
	}

	if err := execJSFunc(vm, "teardown", false); err != nil {
		return err
	}

	return nil
}

func unwrapOptions(optionsValue js.Value) (lib.Options, error) {
	var options lib.Options

	data, err := json.MarshalIndent(optionsValue.Export(), "", "  ")
	if err != nil {
		return lib.Options{}, fmt.Errorf("error parsing script options: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err = dec.Decode(&options); err != nil {
		return lib.Options{}, fmt.Errorf("error while unmarshalling options: %w", err)
	}

	noopLogger := logrus.New()
	noopLogger.SetOutput(io.Discard)

	// Populate options as k6 do, so we execute exact functions as k6 do.
	// It adds default scenario if no other present, default "exec"s and etc.
	// NOTE: Unfortunately there is no exported k6 function
	// to make "consolidated" options (with cli args, envs, and config file).
	options, err = executor.DeriveScenariosFromShortcuts(options, noopLogger)
	if err != nil {
		return lib.Options{}, fmt.Errorf("failed to process k6 options: %w", err)
	}

	return options, nil
}

// createVM creates and configures a new sobek VM instance.
func createVM() *js.Runtime {
	vm := js.New()
	vm.SetParserOptions(parser.IsModule)
	vm.SetFieldNameMapper(js.UncapFieldNameMapper())

	return vm
}

// for [xk6air.DriverWrapper].
type driverStub struct {
	drivers *[]*stroppy.DriverConfig
}

func (*driverStub) RunQuery(string, map[string]any) (*driver.QueryResult, error) {
	return &driver.QueryResult{
		Stats: &stats.Query{},
		Rows:  &rowsStub{},
	}, nil
}

func (*driverStub) InsertValuesBin([]byte, int64) (*stats.Query, error) {
	return &stats.Query{}, nil
}

func (d *driverStub) Setup(configBytes []byte, _ func()) {
	dc := &stroppy.DriverConfig{}
	if err := proto.Unmarshal(configBytes, dc); err == nil {
		*d.drivers = append(*d.drivers, dc)
	}
}

func (*driverStub) Begin(int32) (*txStub, error) {
	return &txStub{}, nil
}

// txStub implements a no-op transaction for the probe VM.
type txStub struct{}

func (t *txStub) RunQuery(string, map[string]any) (*driver.QueryResult, error) {
	return &driver.QueryResult{
		Stats: &stats.Query{},
		Rows:  &rowsStub{},
	}, nil
}

func (*txStub) Commit() error   { return nil }
func (*txStub) Rollback() error { return nil }

// rowsStub implements driver.Rows for the probe VM. It pretends there is
// exactly one row containing a single zero, so queryRow/queryValue never
// return undefined and the workload body can execute past its null-row
// defensive checks (e.g. `if (!distRow) throw ...`).
type rowsStub struct {
	consumed bool
}

func (*rowsStub) Columns() []string { return []string{} }
func (r *rowsStub) Next() bool {
	if r.consumed {
		return false
	}

	r.consumed = true

	return true
}
func (*rowsStub) Values() []any       { return []any{int64(0)} }
func (*rowsStub) ReadAll(int) [][]any { return [][]any{{int64(0)}} }
func (*rowsStub) Err() error          { return nil }
func (*rowsStub) Close() error        { return nil }

type genStub struct{}

// Next returns a non-nil numeric value so TS loops like
// `for (i=1; i<=ol_cnt; i++)` actually iterate at least once, giving the
// probe a chance to register SQL queries that live inside those loops.
func (*genStub) Next() any { return int64(1) }

type groupGenStub struct{}

func (*groupGenStub) Next() any { return []any{} }

// pickerStub executes ALL supplied workload candidates (ignoring weights and
// errors) so that every function's SQL sections get registered, not only the
// first one. It then returns a JS no-op so the caller's `workload()` invocation
// is a harmless second call.
type pickerStub struct {
	vm *js.Runtime
}

func (g *pickerStub) callAll(a []js.Value) {
	for _, v := range a {
		if fn, ok := js.AssertFunction(v); ok {
			_, _ = fn(js.Undefined()) //nolint:errcheck // probe-side, swallow workload throws
		}
	}
}

func (g *pickerStub) noop() js.Value {
	return g.vm.ToValue(func() {})
}

func (g *pickerStub) Pick(a []js.Value) (js.Value, error) {
	g.callAll(a)

	return g.noop(), nil
}

func (g *pickerStub) PickWeighted(a []js.Value, _ []float64) (js.Value, error) {
	g.callAll(a)

	return g.noop(), nil
}

func newPickerStubFactory(vm *js.Runtime) func(uint64) *pickerStub {
	return func(uint64) *pickerStub { return &pickerStub{vm: vm} }
}

type Mocks []struct {
	name  string
	value any
}

func (m Mocks) Set(vm *js.Runtime) error {
	for _, kv := range m {
		if err := vm.Set(kv.name, kv.value); err != nil {
			return fmt.Errorf("failed to set %q for runtime: %w", kv.name, err)
		}
	}

	return nil
}

// prepareVMEnvironment sets up all mocks, polyfills, and globals needed for script execution.
func prepareVMEnvironment(vm *js.Runtime, probeprint *Probeprint) error {
	if err := injectEncoderPolyfill(vm); err != nil {
		return fmt.Errorf("failed to inject encoder polyfill: %w", err)
	}

	newDriverStub := func() any {
		if err := (Mocks{
			// imports from helpers.ts
			{"Step", stepSpy(vm, &probeprint.Steps)},
		}.Set(vm)); err != nil {
			return err
		}

		return &driverStub{drivers: &probeprint.Drivers}
	}

	if err := (Mocks{
		// k6 mocks
		{"__ENV", spyProxyObject(vm, vm.NewObject(), &probeprint.Envs)},
		{"open", func(string) string { return "" }},
		{"console", consoleMock(vm)},
		// k6/metrics
		{"Trend", metricsDummy(vm)},
		{"Rate", metricsDummy(vm)},
		{"Counter", metricsDummy(vm)},
		// TODO: what if user will use other default modules and their functions?

		// k6/x/stroppy defines
		{"NewDriver", newDriverStub},
		{"NewGeneratorByRuleBin", func() any { return &genStub{} }},
		{"NewGroupGeneratorByRulesBin", func() any { return &groupGenStub{} }},
		{"Teardown", func(any) {}},
		{"NotifyStep", notifyStepSpy(&probeprint.Steps)},
		// TODO: research. Some esbuild name resolution artifact, probably
		{"NotifyStep2", notifyStepSpy(&probeprint.Steps)},
		{"NewPicker", newPickerStubFactory(vm)},
		{"DeclareEnv", declareEnvSpy(&probeprint.EnvDeclarations)},
		{"DeclareDriverSetup", declareDriverSetupSpy(&probeprint.DriverSetups)},
		{"Once", func(x any) any { return x }},

		{"parse_sql_with_sections", parseSectionsSpy(&probeprint.SQLSections)},
		{"parse_sql", parseSpy(&probeprint.SQLSections)},
	}.Set(vm)); err != nil {
		return fmt.Errorf("error while applying mocks to runtime: %w", err)
	}

	return nil
}

func declareEnvSpy(decls *[]EnvDeclaration) func([]string, string, string) {
	return func(names []string, default_ string, description string) {
		*decls = append(*decls, EnvDeclaration{
			Names:       names,
			Default:     default_,
			Description: description,
		})
	}
}

func declareDriverSetupSpy(setups *[]DriverSetupDecl) func(int, map[string]any) {
	return func(index int, defaults map[string]any) {
		*setups = append(*setups, DriverSetupDecl{
			Index:    index,
			Defaults: defaults,
		})
	}
}

func notifyStepSpy(steps *[]string) func(string, any) {
	return func(s string, a any) {
		if !slices.Contains(*steps, s) {
			*steps = append(*steps, s)
		}
	}
}

type ParsedQuery struct {
	Name   string
	SQL    string
	Type   string
	Params []string
}

func parseSectionsSpy(
	sections *[]SQLSection,
) func(string, any) func(*string, *string) any {
	return func(string, any) func(*string, *string) any {
		return func(sectionName *string, queryName *string) any {
			if sectionName != nil && *sectionName != "" {
				var section *SQLSection

				i := slices.IndexFunc(*sections,
					func(s SQLSection) bool { return s.Name == *sectionName },
				)
				if i == -1 {
					*sections = append(*sections, SQLSection{Name: *sectionName})
					i = len(*sections) - 1
				}

				section = &(*sections)[i]

				queries := &section.Queries
				if queryName != nil && *queryName != "" {
					j := slices.IndexFunc(*queries,
						func(s SQLQuery) bool { return s.Name == *queryName },
					)
					if j == -1 {
						*queries = append(*queries, SQLQuery{Name: *queryName})
					}

					return ParsedQuery{}
				}

				return []ParsedQuery{}
			}

			return nil // TODO: proxy object
		}
	}
}

func parseSpy(
	sections *[]SQLSection,
) func(string, any) func(*string) any {
	return func(string, any) func(*string) any {
		return func(queryName *string) any {
			*sections = append(*sections, SQLSection{Name: ""})
			i := len(*sections) - 1

			section := &(*sections)[i]

			queries := &section.Queries
			if queryName != nil && *queryName != "" {
				j := slices.IndexFunc(*queries,
					func(s SQLQuery) bool { return s.Name == *queryName },
				)
				if j == -1 {
					*queries = append(*queries, SQLQuery{Name: *queryName})
				}

				return ParsedQuery{}
			}

			return []ParsedQuery{}
		}
	}
}

func stepSpy(vm *js.Runtime, steps *[]string) *js.Object {
	fn := func(name string, lambda js.Callable) any {
		*steps = append(*steps, name)

		if lambda != nil {
			v, _ := lambda(js.Undefined())

			return v
		}

		return nil
	}
	fnObj := vm.ToValue(fn).ToObject(vm)
	_ = fnObj.Set("begin", notifyStepSpy(steps))
	_ = fnObj.Set("end", notifyStepSpy(steps))

	return fnObj
}

func spyProxyObject(
	vm *js.Runtime,
	obj *js.Object,
	accessedProperties *[]string,
) js.Proxy {
	proxy := vm.NewProxy(
		obj,
		&js.ProxyTrapConfig{
			Get: func(
				_ *js.Object, property string, _ js.Value,
			) (value js.Value) {
				*accessedProperties = append(*accessedProperties, property)

				return js.Undefined()
			},
		},
	)

	return proxy
}

func metricsDummy(rt *js.Runtime) *js.Object {
	src := `
	function MyDummy() {}
	MyDummy.prototype.constructor = MyDummy;
	MyDummy.prototype.add = function() {};
	MyDummy;
`
	val, _ := rt.RunString(src)

	return val.ToObject(rt)
}

// setupConsoleMock sets up a no-op console object.
func consoleMock(vm *js.Runtime) *js.Object {
	console := vm.NewObject()
	noOp := func(js.FunctionCall) js.Value { return js.Undefined() }

	_ = console.Set("log", noOp)
	_ = console.Set("warn", noOp)
	_ = console.Set("error", noOp)

	return console
}

//go:embed encodersPolyfill.js
var encodersDefPolyfill string

// injectEncoderPolyfill injects TextEncoder/TextDecoder polyfill into the VM.
func injectEncoderPolyfill(vm *js.Runtime) error {
	// Minified TextEncoder/TextDecoder polyfill check: https://github.com/anonyco/FastestSmallestTextEncoderDecoder
	_, err := vm.RunString(encodersDefPolyfill)

	return err
}
