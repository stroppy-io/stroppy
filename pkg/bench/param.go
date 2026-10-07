package bench

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ParamInputs contains explicitly supplied typed inputs. Environment is read only
// for declared parameter names; unrelated process variables are ignored.
type ParamInputs struct {
	CLI            map[string]string
	RunConfig      map[string]json.RawMessage
	WorkloadConfig map[string]json.RawMessage
}

// ParamSource identifies the winning input channel.
type ParamSource string

const (
	ParamSourceDefault    ParamSource = "default"
	ParamSourceCLI        ParamSource = "cli"
	ParamSourceProcessEnv ParamSource = "process-env"
	ParamSourceConfig     ParamSource = "config"
)

// ParamInfo is copied provenance for one resolved declaration.
type ParamInfo struct {
	Name     string
	Source   ParamSource
	Spelling string
}

func (p ParamInfo) Explicit() bool { return p.Source != ParamSourceDefault }

// ParamType is a discoverable parameter value type.
type ParamType string

const (
	ParamTypeString   ParamType = "string"
	ParamTypeBool     ParamType = "bool"
	ParamTypeInt      ParamType = "int"
	ParamTypeInt64    ParamType = "int64"
	ParamTypeUint64   ParamType = "uint64"
	ParamTypeFloat64  ParamType = "float64"
	ParamTypeDuration ParamType = "duration"
)

// ParamScope identifies the typed configuration object owning a parameter.
type ParamScope string

const (
	ParamScopeRun      ParamScope = "run"
	ParamScopeWorkload ParamScope = "workload"
)

// ParamSchema describes one declaration without requiring execution.
type ParamSchema struct {
	Name               string
	Flag               string
	Scope              ParamScope
	Type               ParamType
	Description        string
	Default            any
	DefaultDescription string
	Env                string
	Config             string
	Aliases            []string
	Constraints        []Constraint
}

// Constraint describes a discoverable parameter restriction.
type Constraint struct {
	Kind  string `json:"kind"`
	Value any    `json:"value"`
}

type resolvedParam struct {
	name     string
	scope    ParamScope
	value    any
	source   ParamSource
	spelling string
}

// Description is a copied observation of a test, not an exhaustive runtime graph.
type Description struct {
	Name    string
	Params  []ParamSchema
	Steps   []StepDescription
	Drivers []DriverDescription
	Values  map[string]ResolvedParameter
	Metrics []MetricSchema
}

// ResolvedParameter is a copied effective value and provenance.
type ResolvedParameter struct {
	Value any
	Info  ParamInfo
}

// ParamOption modifies declaration metadata and optional constraints.
type (
	ParamOption interface {
		apply(descriptor *paramDescriptor)
	}
	paramOption func(*paramDescriptor)
)

func (o paramOption) apply(d *paramDescriptor) { o(d) }

// Aliases adds alternate parameter names with the same flag/env/config projections.
func Aliases(names ...string) ParamOption {
	names = slices.Clone(names)

	return paramOption(func(d *paramDescriptor) { d.aliases = append(d.aliases, names...) })
}

// DerivedDefault describes a contextual fallback instead of a fixed schema value.
func DerivedDefault(description string) ParamOption {
	return paramOption(func(d *paramDescriptor) { d.defaultDescription = description })
}

// Min and Max apply inclusive numeric or duration bounds.
func Min[T ~int | ~int64 | ~uint64 | ~float64](value T) ParamOption { return bound("min", value) }
func Max[T ~int | ~int64 | ~uint64 | ~float64](value T) ParamOption { return bound("max", value) }
func bound(kind string, value any) ParamOption {
	return paramOption(func(d *paramDescriptor) { d.constraints = append(d.constraints, Constraint{kind, value}) })
}

// OneOf restricts a declaration to the supplied comparable values.
func OneOf[T comparable](values ...T) ParamOption {
	values = slices.Clone(values)

	return paramOption(func(d *paramDescriptor) { d.constraints = append(d.constraints, Constraint{"one-of", values}) })
}

type paramDescriptor struct {
	name               string
	scope              ParamScope
	typ                ParamType
	description        string
	defaultValue       any
	defaultDescription string
	aliases            []string
	constraints        []Constraint
}

// Def scopes one observation or execution replay. Subspaces may be passed to
// ordinary helpers. Actions run only through Execution.
type Def struct {
	Param        ParamDeclarations
	Execution    Execution
	Drivers      DriverDeclarations
	Queries      QueryFiles
	Metrics      MetricDeclarations
	Report       ReportDeclarations
	inputs       ParamInputs
	defaultsOnly bool
	descriptors  []paramDescriptor
	resolved     []resolvedParam
	names        map[string]string
	projections  map[string]string
	environment  map[string]string
	reports      []reportDefinition
	scope        ParamScope
}

// ParamDeclarations owns declaration, resolution, and copied provenance.
type ParamDeclarations struct{ def *Def }

var paramNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

func newDef(inputs ParamInputs, defaultsOnly bool) *Def {
	d := &Def{
		inputs:       cloneParamInputs(inputs),
		defaultsOnly: defaultsOnly,
		scope:        ParamScopeWorkload,
		names:        map[string]string{},
		projections:  map[string]string{},
	}
	d.Param.def = d
	d.Execution.def = d
	d.Drivers.def = d
	d.Metrics.def = d
	d.Report.def = d
	d.environment = map[string]string{}

	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			d.environment[name] = value
		}
	}

	return d
}

func cloneParamInputs(in ParamInputs) ParamInputs {
	out := ParamInputs{
		CLI:            map[string]string{},
		RunConfig:      map[string]json.RawMessage{},
		WorkloadConfig: map[string]json.RawMessage{},
	}
	for k, v := range in.CLI {
		out.CLI[k] = v
	}

	for k, v := range in.RunConfig {
		out.RunConfig[k] = slices.Clone(v)
	}

	for k, v := range in.WorkloadConfig {
		out.WorkloadConfig[k] = slices.Clone(v)
	}

	return out
}

func (p ParamDeclarations) String(name, fallback, description string, opts ...ParamOption) (string, ParamInfo) {
	return p.Declare(name, fallback, description, opts...)
}

func (p ParamDeclarations) Bool(name string, fallback bool, description string, opts ...ParamOption) (bool, ParamInfo) {
	return p.Declare(name, fallback, description, opts...)
}

func (p ParamDeclarations) Int(name string, fallback int, description string, opts ...ParamOption) (int, ParamInfo) {
	return p.Declare(name, fallback, description, opts...)
}

func (p ParamDeclarations) Int64(
	name string,
	fallback int64,
	description string,
	opts ...ParamOption,
) (int64, ParamInfo) {
	return p.Declare(name, fallback, description, opts...)
}

func (p ParamDeclarations) Uint64(
	name string,
	fallback uint64,
	description string,
	opts ...ParamOption,
) (uint64, ParamInfo) {
	return p.Declare(name, fallback, description, opts...)
}

func (p ParamDeclarations) Float64(
	name string,
	fallback float64,
	description string,
	opts ...ParamOption,
) (float64, ParamInfo) {
	return p.Declare(name, fallback, description, opts...)
}

func (p ParamDeclarations) Duration(
	name string,
	fallback time.Duration,
	description string,
	opts ...ParamOption,
) (time.Duration, ParamInfo) {
	return p.Declare(name, fallback, description, opts...)
}

// Declare resolves a supported scalar type immediately. Invalid declarations or
// winning inputs panic with ValidationError; application boundaries return it.
func (p ParamDeclarations) Declare[T any](
	name string,
	fallback T,
	description string,
	opts ...ParamOption,
) (T, ParamInfo) {
	d := p.def
	if d == nil {
		invalid("parameter "+name, inputError("nil declaration context"))
	}

	typ := parameterType(reflect.TypeFor[T]())
	desc := paramDescriptor{
		name:         name,
		scope:        d.scope,
		typ:          typ,
		description:  description,
		defaultValue: fallback,
	}

	for _, opt := range opts {
		if opt == nil {
			invalid(name, inputError("nil parameter option"))
		}

		opt.apply(&desc)
	}

	d.register(&desc)
	validateConstraints(&desc, fallback)

	info := ParamInfo{Name: name, Source: ParamSourceDefault}
	value := fallback

	if !d.defaultsOnly {
		raw, source, spelling, isJSON, found := d.pick(&desc)
		if found {
			parsed, err := parseParameter[T](raw, isJSON)
			if err != nil {
				invalid(fmt.Sprintf("parameter %q from %s (%s)", name, source, spelling), err)
			}

			value = parsed
			info.Source, info.Spelling = source, spelling
		}
	}

	validateConstraints(&desc, value)
	d.resolved = append(d.resolved, resolvedParam{
		name,
		desc.scope,
		reportParamValue(value),
		info.Source,
		info.Spelling,
	})

	return value, info
}

func (p ParamDeclarations) Var[T any](
	dst *T,
	name string,
	fallback T,
	description string,
	opts ...ParamOption,
) ParamInfo {
	if dst == nil {
		invalid(name, inputError("nil parameter destination"))
	}

	value, info := p.Declare(name, fallback, description, opts...)
	*dst = value

	return info
}

func (p ParamDeclarations) IntVar(
	dst *int,
	name string,
	fallback int,
	description string,
	opts ...ParamOption,
) ParamInfo {
	return p.Var(dst, name, fallback, description, opts...)
}

func (p ParamDeclarations) Int64Var(
	dst *int64,
	name string,
	fallback int64,
	description string,
	opts ...ParamOption,
) ParamInfo {
	return p.Var(dst, name, fallback, description, opts...)
}

func (p ParamDeclarations) Uint64Var(
	dst *uint64,
	name string,
	fallback uint64,
	description string,
	opts ...ParamOption,
) ParamInfo {
	return p.Var(dst, name, fallback, description, opts...)
}

func (p ParamDeclarations) StringVar(dst *string, name, fallback, description string, opts ...ParamOption) ParamInfo {
	return p.Var(dst, name, fallback, description, opts...)
}

func (p ParamDeclarations) BoolVar(
	dst *bool,
	name string,
	fallback bool,
	description string,
	opts ...ParamOption,
) ParamInfo {
	return p.Var(dst, name, fallback, description, opts...)
}

func (p ParamDeclarations) Float64Var(
	dst *float64,
	name string,
	fallback float64,
	description string,
	opts ...ParamOption,
) ParamInfo {
	return p.Var(dst, name, fallback, description, opts...)
}

func (p ParamDeclarations) DurationVar(
	dst *time.Duration,
	name string,
	fallback time.Duration,
	description string,
	opts ...ParamOption,
) ParamInfo {
	return p.Var(dst, name, fallback, description, opts...)
}

func parameterType(t reflect.Type) ParamType {
	if t == reflect.TypeFor[time.Duration]() {
		return ParamTypeDuration
	}

	switch t.Kind() {
	case reflect.String:
		return ParamTypeString
	case reflect.Bool:
		return ParamTypeBool
	case reflect.Int:
		return ParamTypeInt
	case reflect.Int64:
		return ParamTypeInt64
	case reflect.Uint64:
		return ParamTypeUint64
	case reflect.Float64:
		return ParamTypeFloat64
	default:
		invalid("parameter type", inputError("unsupported %s", t))

		return ""
	}
}

//nolint:nestif // JSON and text inputs converge on one typed value.
func parseParameter[T any](text string, isJSON bool) (T, error) {
	var value T

	t := reflect.TypeFor[T]()

	if isJSON {
		if bytes.Equal(bytes.TrimSpace([]byte(text)), []byte("null")) {
			return value, inputError("null is not allowed")
		}

		if t == reflect.TypeFor[time.Duration]() {
			var duration string
			if err := json.Unmarshal([]byte(text), &duration); err != nil {
				return value, err
			}

			text = duration
		} else {
			err := json.Unmarshal([]byte(text), &value)

			return value, err
		}
	}

	target := reflect.ValueOf(&value).Elem()

	var err error

	if t == reflect.TypeFor[time.Duration]() {
		var parsed time.Duration

		parsed, err = time.ParseDuration(text)
		target.SetInt(int64(parsed))
	} else {
		//nolint:exhaustive // parameterType rejects all unsupported kinds before parsing.
		switch target.Kind() {
		case reflect.String:
			target.SetString(text)
		case reflect.Bool:
			var parsed bool

			parsed, err = strconv.ParseBool(text)
			target.SetBool(parsed)
		case reflect.Int, reflect.Int64:
			var parsed int64

			parsed, err = strconv.ParseInt(text, 10, target.Type().Bits())
			target.SetInt(parsed)
		case reflect.Uint64:
			var parsed uint64

			parsed, err = strconv.ParseUint(text, 10, 64)
			target.SetUint(parsed)
		case reflect.Float64:
			var parsed float64

			parsed, err = strconv.ParseFloat(text, 64)
			target.SetFloat(parsed)
		}
	}

	return value, err
}

func (d *Def) register(desc *paramDescriptor) {
	for _, name := range append([]string{desc.name}, desc.aliases...) {
		if !paramNamePattern.MatchString(name) {
			invalid("parameter "+desc.name, inputError("invalid name %q: use lower-case kebab-case", name))
		}

		if owner, ok := d.names[name]; ok {
			invalid("parameter "+desc.name, inputError("name %q already belongs to %q", name, owner))
		}

		for _, key := range []string{"env:" + kebabToEnv(name), "config:" + kebabToCamel(name)} {
			if owner, ok := d.projections[key]; ok {
				invalid("parameter "+desc.name, inputError("projection %q collides with %q", key, owner))
			}

			d.projections[key] = desc.name
		}

		d.names[name] = desc.name
	}

	d.descriptors = append(d.descriptors, *desc)
}

func (d *Def) pick(desc *paramDescriptor) (raw string, source ParamSource, spelling string, isJSON, found bool) {
	names := append([]string{desc.name}, desc.aliases...)
	for _, name := range names {
		if value, ok := d.inputs.CLI[name]; ok {
			return value, ParamSourceCLI, name, false, true
		}
	}

	for _, name := range names {
		env := kebabToEnv(name)
		if value, ok := lookupEnvironment(d.environment, env); ok {
			return value, ParamSourceProcessEnv, env, false, true
		}
	}

	cfg := d.inputs.WorkloadConfig
	if desc.scope == ParamScopeRun {
		cfg = d.inputs.RunConfig
	}

	for _, name := range names {
		key := kebabToCamel(name)
		if value, ok := cfg[key]; ok {
			return string(value), ParamSourceConfig, key, true, true
		}
	}

	return "", ParamSourceDefault, "", false, false
}

//nolint:gocognit // bounds and choice checks share one declaration boundary.
func validateConstraints(desc *paramDescriptor, value any) {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Float64 && (math.IsInf(v.Float(), 0) || math.IsNaN(v.Float())) {
		invalid(desc.name, inputError("must be finite"))
	}

	seen := map[string]bool{}
	for _, constraint := range desc.constraints {
		if seen[constraint.Kind] {
			invalid(desc.name, inputError("duplicate constraint"))
		}

		seen[constraint.Kind] = true
		switch constraint.Kind {
		case "min", "max":
			comparison, err := compareNumbers(v, reflect.ValueOf(constraint.Value))
			if err != nil {
				invalid(desc.name, err)
			}

			if constraint.Kind == "min" && comparison < 0 {
				invalid(desc.name, inputError("must be at least %v", constraint.Value))
			}

			if constraint.Kind == "max" && comparison > 0 {
				invalid(desc.name, inputError("must be at most %v", constraint.Value))
			}
		case "one-of":
			values := reflect.ValueOf(constraint.Value)
			if values.Len() == 0 || values.Type().Elem() != v.Type() {
				invalid(desc.name, inputError("incompatible or empty choices"))
			}

			found := false

			for index := range values.Len() {
				if reflect.DeepEqual(values.Index(index).Interface(), value) {
					found = true

					break
				}
			}

			if !found {
				invalid(desc.name, inputError("must be one of %v", constraint.Value))
			}
		}
	}
}

func compareNumbers(left, right reflect.Value) (int, error) {
	toNumber := func(value reflect.Value) (*big.Rat, error) {
		number := new(big.Rat)

		switch value.Kind() {
		case reflect.Int, reflect.Int64:
			return number.SetInt64(value.Int()), nil
		case reflect.Uint64:
			return number.SetInt(new(big.Int).SetUint64(value.Uint())), nil
		case reflect.Float64:
			if number.SetFloat64(value.Float()) == nil {
				return nil, inputError("bound must be finite")
			}

			return number, nil
		default:
			return nil, inputError("numeric constraint on nonnumeric parameter")
		}
	}

	a, err := toNumber(left)
	if err != nil {
		return 0, err
	}

	b, err := toNumber(right)
	if err != nil {
		return 0, err
	}

	return a.Cmp(b), nil
}

func lookupEnvironment(values map[string]string, name string) (string, bool) {
	value, found := values[name]

	return value, found
}

//nolint:gocognit // all supplied scopes are checked before actions.
func (d *Def) finish() error {
	var failures []error

	keys := make([]string, 0, len(d.inputs.CLI))
	for name := range d.inputs.CLI {
		keys = append(keys, name)
	}

	slices.Sort(keys)

	for _, name := range keys {
		if _, ok := d.names[name]; !ok {
			failures = append(failures, inputError("unknown CLI parameter %q", name))
		}
	}

	for _, scope := range []ParamScope{ParamScopeRun, ParamScopeWorkload} {
		values := d.inputs.WorkloadConfig
		if scope == ParamScopeRun {
			values = d.inputs.RunConfig
		}

		allowed := map[string]bool{}

		for index := range d.descriptors {
			desc := &d.descriptors[index]
			if desc.scope == scope {
				for _, name := range append([]string{desc.name}, desc.aliases...) {
					allowed[kebabToCamel(name)] = true
				}
			}
		}

		keys = keys[:0]
		for name := range values {
			keys = append(keys, name)
		}

		slices.Sort(keys)

		for _, name := range keys {
			if !allowed[name] {
				failures = append(failures, inputError("unknown %s config parameter %q", scope, name))
			}
		}
	}

	return errors.Join(failures...)
}

func (d *Def) schema() []ParamSchema {
	out := make([]ParamSchema, 0, len(d.descriptors))
	for index := range d.descriptors {
		desc := &d.descriptors[index]

		fallback := reportParamValue(desc.defaultValue)
		if desc.defaultDescription != "" {
			fallback = nil
		}

		out = append(out, ParamSchema{
			Name:               desc.name,
			Flag:               "--" + desc.name,
			Scope:              desc.scope,
			Type:               desc.typ,
			Description:        desc.description,
			Default:            fallback,
			DefaultDescription: desc.defaultDescription,
			Env:                kebabToEnv(desc.name),
			Config:             kebabToCamel(desc.name),
			Aliases:            slices.Clone(desc.aliases),
			Constraints:        slices.Clone(desc.constraints),
		})
	}

	slices.SortFunc(out, func(a, b ParamSchema) int {
		if a.Scope != b.Scope {
			return strings.Compare(string(a.Scope), string(b.Scope))
		}

		return strings.Compare(a.Name, b.Name)
	})

	return out
}

func reportParamValue(value any) any {
	if v, ok := value.(time.Duration); ok {
		return v.String()
	}

	return value
}
func kebabToEnv(name string) string { return strings.ToUpper(strings.ReplaceAll(name, "-", "_")) }
func kebabToCamel(name string) string {
	parts := strings.Split(name, "-")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}

	return strings.Join(parts, "")
}
