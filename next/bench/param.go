package bench

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Source names the provenance of a param's resolved value, in increasing
// precedence (Default is the declared fallback, CLI wins over everything).
type Source int

const (
	SourceDefault Source = iota // the declared default
	SourceConfig                // a flat JSON config file
	SourceEnv                   // a process-env variable
	SourceCLI                   // a --name=val flag
)

// String renders the source for the probe schema and diagnostics.
func (s Source) String() string {
	switch s {
	case SourceCLI:
		return "cli"
	case SourceEnv:
		return "env"
	case SourceConfig:
		return "config"
	default:
		return "default"
	}
}

// ParamSchema is one param's probe description and the unit of the --help
// registry output. It is the single projection of a declared param: defined
// once, surfaced to probe, --help, and introspection uniformly (D1).
type ParamSchema struct {
	Name    string `json:"name"`    // registry name; also the --flag and config key
	Env     string `json:"env"`     // env-var name (bare uppercase by default)
	Flag    string `json:"flag"`    // the --name projection
	Config  string `json:"config"`  // the JSON-key projection
	Type    string `json:"type"`    // int|int64|uint64|float64|bool|string|duration
	Help    string `json:"help"`    // one-line description
	Default string `json:"default"` // declared default, rendered as text
	Current string `json:"current"` // resolved value, rendered as text
	Source  string `json:"source"`  // where Current came from
	Standard bool   `json:"-"`       // STANDARD (SDK) group vs TEST (author) group
}

// paramKind labels a param's type for the schema and the parse dispatch.
type paramKind string

const (
	kindInt      paramKind = "int"
	kindInt64    paramKind = "int64"
	kindUint64   paramKind = "uint64"
	kindFloat64  paramKind = "float64"
	kindBool     paramKind = "bool"
	kindString   paramKind = "string"
	kindDuration paramKind = "duration"
)

// paramDecl is one declared param's metadata plus its resolved value. It backs
// both the typed [param] handle and the [ParamSchema] projection.
type paramDecl struct {
	name     string
	env      string
	config   string
	kind     paramKind
	help     string
	defStr   string
	standard bool // STANDARD group (SDK-injected) vs TEST group (author)
	value    any  // resolved typed value
	raw      string
	src      Source
}

// param is the typed handle to one declared param. Construct it through a
// [paramSet] accessor ([paramSet.Int], [paramSet.String], ...); the value is
// resolved from the input bags at construction (immediate-mode: cli > env >
// config > default), so Value returns the real datum and authors can derive and
// branch inline on it. D7's Define callback is the author-facing entry point;
// until it lands the struct-tag adapter and the SDK's own standard params use
// these handles internally.
type param[T any] struct {
	decl *paramDecl
}

// Value returns the resolved value (the declared default when resolution failed
// at construction, so a downstream declare is never poisoned by an earlier
// parse error).
func (p *param[T]) Value() T {
	if p.decl == nil || p.decl.value == nil {
		var z T
		return z
	}
	return p.decl.value.(T)
}

// Source reports where Value came from.
func (p *param[T]) Source() Source {
	if p.decl == nil {
		return SourceDefault
	}
	return p.decl.src
}

// paramOpt reconfigures a param's projections at registration.
type paramOpt func(*paramDecl)

// optEnv overrides the env-var name (default: the bare uppercase of the param
// name). It is the only projection override authors commonly need: a name like
// "warehouses" already projects to --warehouses / config "warehouses" / env
// WAREHOUSES, and optEnv covers the legacy-renamed case (a v5 env name carried
// verbatim into next).
func optEnv(name string) paramOpt {
	return func(d *paramDecl) { d.env = name }
}

// optStandard marks the param as SDK-injected (the STANDARD --help group) rather
// than an author/test param (the TEST group).
func optStandard() paramOpt {
	return func(d *paramDecl) { d.standard = true }
}

// paramSet is the param registry: params declared over one run's resolved input
// bags. Each source is parsed into a bag BEFORE any param is declared, so each
// accessor both registers metadata and resolves the value immediately. The set
// is the single source of truth behind probe, --help, and the typed handles.
type paramSet struct {
	cli    map[string]string          // --name=val flags
	env    func(string) string        // process env lookup
	cfg    map[string]json.RawMessage // flat JSON config, keyed by param name

	order   []*paramDecl
	byName  map[string]*paramDecl
	cliUsed map[string]bool

	err error // first resolution error (recorded, not thrown, so probe can still run)
}

// newParamSet builds a registry over the three input sources. Any source may be
// nil (treated as empty).
func newParamSet(cli map[string]string, env func(string) string, cfg map[string]json.RawMessage) *paramSet {
	return &paramSet{
		cli: cli, env: env, cfg: cfg,
		byName:  make(map[string]*paramDecl),
		cliUsed: make(map[string]bool),
	}
}

// declare registers d's metadata and returns the fresh decl, or an error if the
// name is already taken. It does not resolve the value.
func (s *paramSet) declare(name string, kind paramKind, help, defStr string, opts []paramOpt) (*paramDecl, error) {
	if _, dup := s.byName[name]; dup {
		return nil, fmt.Errorf("bench: param %q declared twice", name)
	}
	d := &paramDecl{
		name:   name,
		kind:   kind,
		help:   help,
		defStr: defStr,
		env:    strings.ToUpper(name),
		config: name,
	}
	for _, o := range opts {
		o(d)
	}
	s.order = append(s.order, d)
	s.byName[name] = d
	return d, nil
}

// pick resolves the source for d: cli > env > config > default. For config it
// returns the [json.RawMessage] text and fromCfg=true; otherwise the raw string.
func (s *paramSet) pick(d *paramDecl) (raw string, fromCfg bool, src Source, found bool) {
	if s.cli != nil {
		if v, ok := s.cli[d.name]; ok {
			s.cliUsed[d.name] = true
			return v, false, SourceCLI, true
		}
	}
	if s.env != nil {
		if e := s.env(d.env); e != "" {
			return e, false, SourceEnv, true
		}
	}
	if s.cfg != nil {
		if rm, ok := s.cfg[d.config]; ok {
			return string(rm), true, SourceConfig, true
		}
	}
	return "", false, SourceDefault, false
}

// fail records the first resolution error. Declaring continues so a probe can
// still report the full catalog; the caller checks [paramSet.Err] after.
func (s *paramSet) fail(err error) {
	if s.err == nil {
		s.err = err
	}
}

// Err returns the first resolution error recorded during declaration, or nil.
func (s *paramSet) Err() error { return s.err }

// checkUnknown reports a --flag that no registered param consumed (a typo),
// listing the offending names.
func (s *paramSet) checkUnknown() error {
	var unknown []string
	for k := range s.cli {
		if !s.cliUsed[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	return fmt.Errorf("bench: unknown flag(s) --%s", strings.Join(unknown, ", --"))
}

// Schema returns every declared param in registration order.
func (s *paramSet) Schema() []ParamSchema {
	out := make([]ParamSchema, 0, len(s.order))
	for _, d := range s.order {
		out = append(out, d.schema())
	}
	return out
}

func (d *paramDecl) schema() ParamSchema {
	return ParamSchema{
		Name: d.name, Env: d.env, Flag: "--" + d.name, Config: d.config,
		Type: string(d.kind), Help: d.help, Default: d.defStr,
		Current: d.raw, Source: d.src.String(), Standard: d.standard,
	}
}

// resolveParam is the shared engine behind every typed accessor: declare, pick
// the winning source, parse, and store both the typed value and a display
// string. It is a top-level generic (not a method) because Go methods cannot
// carry their own type parameters.
//
// Config values carry their type in JSON: a string value ("5s", "auto") reuses
// the same text-parse path as cli/env, so authors write durations the same way
// in a file as on the command line; any other JSON value (number/bool) unmarshals
// directly into T.
func resolveParam[T any](s *paramSet, name string, kind paramKind, def T, defStr, help string, parse func(string) (T, error), opts []paramOpt) *param[T] {
	d, err := s.declare(name, kind, help, defStr, opts)
	if err != nil {
		s.fail(err)
		return &param[T]{}
	}
	raw, fromCfg, src, found := s.pick(d)
	d.src = src
	switch {
	case !found:
		d.value = def
		d.raw = defStr
	case fromCfg && len(raw) > 0 && raw[0] == '"':
		// A JSON-string config value carries text the same way cli/env do, so a
		// duration is written "5s" in the file exactly as on the command line.
		var text string
		if e := json.Unmarshal([]byte(raw), &text); e != nil {
			s.fail(fmt.Errorf("bench: param %q: %w", name, e))
			d.value, d.raw = def, defStr
			return &param[T]{d}
		}
		v, e := parse(text)
		if e != nil {
			s.fail(fmt.Errorf("bench: param %q: %w", name, e))
			d.value, d.raw = def, defStr
			return &param[T]{d}
		}
		d.value, d.raw = v, render(v)
	case fromCfg:
		// A non-string JSON config value (number/bool) unmarshals directly into T.
		var v T
		if e := json.Unmarshal([]byte(raw), &v); e != nil {
			s.fail(fmt.Errorf("bench: param %q: %w", name, e))
			d.value, d.raw = def, defStr
			return &param[T]{d}
		}
		d.value, d.raw = v, render(v)
	default: // cli or env: parse the raw text
		v, e := parse(raw)
		if e != nil {
			s.fail(fmt.Errorf("bench: param %q: %w", name, e))
			d.value, d.raw = def, defStr
			return &param[T]{d}
		}
		d.value, d.raw = v, render(v)
	}
	return &param[T]{d}
}

// render is the canonical text form of a resolved value for the schema's Current
// field. time.Duration renders via its own String ("5s"); everything else via
// fmt's default verb (numbers without quotes, bools as true/false, strings raw).
func render(v any) string {
	if d, ok := v.(time.Duration); ok {
		return d.String()
	}
	return fmt.Sprintf("%v", v)
}

// The typed accessors. Each returns a handle whose Value is already resolved
// (immediate-mode); the default is also the fallback when a parse fails.

func (s *paramSet) Int(name string, def int, help string, opts ...paramOpt) *param[int] {
	return resolveParam(s, name, kindInt, def, strconv.Itoa(def), help, strconv.Atoi, opts)
}

func (s *paramSet) Int64(name string, def int64, help string, opts ...paramOpt) *param[int64] {
	return resolveParam(s, name, kindInt64, def, strconv.FormatInt(def, 10), help,
		func(x string) (int64, error) { return strconv.ParseInt(x, 10, 64) }, opts)
}

func (s *paramSet) Uint64(name string, def uint64, help string, opts ...paramOpt) *param[uint64] {
	return resolveParam(s, name, kindUint64, def, strconv.FormatUint(def, 10), help,
		func(x string) (uint64, error) { return strconv.ParseUint(x, 10, 64) }, opts)
}

func (s *paramSet) Float64(name string, def float64, help string, opts ...paramOpt) *param[float64] {
	return resolveParam(s, name, kindFloat64, def, strconv.FormatFloat(def, 'f', -1, 64), help,
		func(x string) (float64, error) { return strconv.ParseFloat(x, 64) }, opts)
}

func (s *paramSet) Bool(name string, def bool, help string, opts ...paramOpt) *param[bool] {
	return resolveParam(s, name, kindBool, def, strconv.FormatBool(def), help, strconv.ParseBool, opts)
}

func (s *paramSet) String(name string, def, help string, opts ...paramOpt) *param[string] {
	return resolveParam(s, name, kindString, def, def, help,
		func(x string) (string, error) { return x, nil }, opts)
}

func (s *paramSet) Duration(name string, def time.Duration, help string, opts ...paramOpt) *param[time.Duration] {
	return resolveParam(s, name, kindDuration, def, def.String(), help, time.ParseDuration, opts)
}
