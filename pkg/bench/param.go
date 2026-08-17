package bench

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ParamInputs keeps typed parameter channels separate from compatibility env channels.
type ParamInputs struct {
	CLI             map[string]string
	LegacyEnv       map[string]string
	RunConfig       map[string]json.RawMessage
	WorkloadConfig  map[string]json.RawMessage
	LegacyConfigEnv map[string]string
}

// ParamSource identifies the channel that supplied a resolved parameter value.
type ParamSource string

const (
	ParamSourceDefault         ParamSource = "default"
	ParamSourceCLI             ParamSource = "cli"
	ParamSourceProcessEnv      ParamSource = "process-env"
	ParamSourceLegacyEnv       ParamSource = "legacy-env"
	ParamSourceConfig          ParamSource = "config"
	ParamSourceLegacyConfigEnv ParamSource = "legacy-config-env"
)

// Param is one typed value resolved for a single workload operation.
type Param[T any] struct {
	value  T
	source ParamSource
}

func (p Param[T]) Value() T            { return p.value }
func (p Param[T]) Source() ParamSource { return p.source }
func (p Param[T]) Explicit() bool      { return p.source != ParamSourceDefault }

// ParamType is the concrete type exposed by a parameter declaration.
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

// ParamScope identifies the config object that owns a declaration.
type ParamScope string

const (
	ParamScopeRun      ParamScope = "run"
	ParamScopeWorkload ParamScope = "workload"
)

// ParamSchema is a copied projection of one parameter declaration.
type ParamSchema struct {
	Name             string
	Flag             string
	Scope            ParamScope
	Type             ParamType
	Description      string
	Default          any
	Env              string
	LegacyEnvAliases []string
	Config           string
}

// Description is the discoverable, default-only schema for a workload.
type Description struct {
	Name   string
	Params []ParamSchema
}

// ParamOption customizes declaration metadata.
type ParamOption interface {
	apply(desc *paramDescriptor) error
}

type paramOption func(*paramDescriptor) error

func (o paramOption) apply(desc *paramDescriptor) error { return o(desc) }

// LegacyEnvAliases accepts old environment names in addition to the projected name.
func LegacyEnvAliases(names ...string) ParamOption {
	aliases := slices.Clone(names)

	return paramOption(func(desc *paramDescriptor) error {
		desc.legacyEnvAliases = append(desc.legacyEnvAliases, aliases...)

		return nil
	})
}

type paramDescriptor struct {
	name             string
	scope            ParamScope
	typ              ParamType
	description      string
	defaultValue     any
	env              string
	legacyEnvAliases []string
	config           string
}

type paramParser[T any] struct {
	text func(string) (T, error)
	raw  func(json.RawMessage) (T, error)
}

// Def is the immediate workload definition and resolution operation.
type Def struct {
	Param ParamDeclarations

	inputs       ParamInputs
	defaultsOnly bool
	scope        ParamScope
	descriptors  []paramDescriptor
	names        map[string]struct{}
	envNames     map[string]string
	errs         []error
}

// ParamDeclarations contains the concrete typed declaration accessors.
type ParamDeclarations struct {
	def *Def
}

var (
	paramNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	envNamePattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

	errNilParamOption        = errors.New("nil parameter option")
	errInvalidParamName      = errors.New("invalid parameter name")
	errReservedParamName     = errors.New("reserved parameter name")
	errDuplicateParamName    = errors.New("duplicate parameter name")
	errInvalidLegacyEnvAlias = errors.New("invalid legacy env alias")
	errProjectedEnvAlias     = errors.New("legacy env alias duplicates its projected env name")
	errDuplicateLegacyAlias  = errors.New("duplicate legacy env alias")
	errDuplicateParamEnvName = errors.New("parameter env name already registered")
	errNullParamValue        = errors.New("null is not allowed")
	errNonFiniteParamValue   = errors.New("must be finite")
	errUnknownCLIParam       = errors.New("unknown CLI parameter")
	errUnknownRunConfigParam = errors.New("unknown run config parameter")
	errUnknownWorkloadConfig = errors.New("unknown workload config parameter")
)

var reservedWorkloadParamNames = map[string]struct{}{
	"driver": {}, "driver-opt": {}, "duration": {}, "env": {}, "executor": {},
	"file": {}, "help": {}, "iterations": {}, "no-steps": {}, "steps": {}, "vus": {},
}

func newDef(inputs ParamInputs, defaultsOnly bool) *Def {
	d := &Def{
		inputs:       cloneParamInputs(inputs),
		defaultsOnly: defaultsOnly,
		scope:        ParamScopeRun,
		names:        make(map[string]struct{}),
		envNames:     make(map[string]string),
	}
	d.Param.def = d

	return d
}

func cloneParamInputs(inputs ParamInputs) ParamInputs {
	return ParamInputs{
		CLI:             cloneStringMap(inputs.CLI),
		LegacyEnv:       cloneStringMap(inputs.LegacyEnv),
		RunConfig:       cloneRawMap(inputs.RunConfig),
		WorkloadConfig:  cloneRawMap(inputs.WorkloadConfig),
		LegacyConfigEnv: cloneStringMap(inputs.LegacyConfigEnv),
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}

	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}

func cloneRawMap(src map[string]json.RawMessage) map[string]json.RawMessage {
	if src == nil {
		return nil
	}

	dst := make(map[string]json.RawMessage, len(src))
	for key, value := range src {
		dst[key] = slices.Clone(value)
	}

	return dst
}

func (p ParamDeclarations) String(
	name, defaultValue, description string,
	opts ...ParamOption,
) Param[string] {
	return declareParam(p.def, name, ParamTypeString, defaultValue, description, paramParser[string]{
		text: func(value string) (string, error) { return value, nil },
		raw:  decodeJSON[string],
	}, opts...)
}

func (p ParamDeclarations) Bool(
	name string,
	defaultValue bool,
	description string,
	opts ...ParamOption,
) Param[bool] {
	return declareParam(p.def, name, ParamTypeBool, defaultValue, description, paramParser[bool]{
		text: strconv.ParseBool,
		raw:  decodeJSON[bool],
	}, opts...)
}

func (p ParamDeclarations) Int(
	name string,
	defaultValue int,
	description string,
	opts ...ParamOption,
) Param[int] {
	return declareParam(p.def, name, ParamTypeInt, defaultValue, description, paramParser[int]{
		text: strconv.Atoi,
		raw:  decodeJSON[int],
	}, opts...)
}

func (p ParamDeclarations) Int64(
	name string,
	defaultValue int64,
	description string,
	opts ...ParamOption,
) Param[int64] {
	return declareParam(p.def, name, ParamTypeInt64, defaultValue, description, paramParser[int64]{
		text: func(value string) (int64, error) { return strconv.ParseInt(value, 10, 64) },
		raw:  decodeJSON[int64],
	}, opts...)
}

func (p ParamDeclarations) Uint64(
	name string,
	defaultValue uint64,
	description string,
	opts ...ParamOption,
) Param[uint64] {
	return declareParam(p.def, name, ParamTypeUint64, defaultValue, description, paramParser[uint64]{
		text: func(value string) (uint64, error) { return strconv.ParseUint(value, 10, 64) },
		raw:  decodeJSON[uint64],
	}, opts...)
}

func (p ParamDeclarations) Float64(
	name string,
	defaultValue float64,
	description string,
	opts ...ParamOption,
) Param[float64] {
	if math.IsInf(defaultValue, 0) || math.IsNaN(defaultValue) {
		p.def.addError(fmt.Errorf("parameter %q default: %w", name, errNonFiniteParamValue))
	}

	return declareParam(p.def, name, ParamTypeFloat64, defaultValue, description, paramParser[float64]{
		text: parseFiniteFloat,
		raw:  decodeFiniteFloat,
	}, opts...)
}

func (p ParamDeclarations) Duration(
	name string,
	defaultValue time.Duration,
	description string,
	opts ...ParamOption,
) Param[time.Duration] {
	return declareParam(p.def, name, ParamTypeDuration, defaultValue, description, paramParser[time.Duration]{
		text: time.ParseDuration,
		raw: func(raw json.RawMessage) (time.Duration, error) {
			value, err := decodeJSON[string](raw)
			if err != nil {
				return 0, err
			}

			return time.ParseDuration(value)
		},
	}, opts...)
}

func declareParam[T any](
	d *Def,
	name string,
	typ ParamType,
	defaultValue T,
	description string,
	parser paramParser[T],
	opts ...ParamOption,
) Param[T] {
	desc := paramDescriptor{
		name:         name,
		scope:        d.scope,
		typ:          typ,
		description:  description,
		defaultValue: defaultValue,
		env:          kebabToEnv(name),
		config:       kebabToCamel(name),
	}

	for _, opt := range opts {
		if opt == nil {
			d.addError(fmt.Errorf("%w for parameter %q", errNilParamOption, name))

			continue
		}

		if err := opt.apply(&desc); err != nil {
			d.addError(fmt.Errorf("parameter %q option: %w", name, err))
		}
	}

	valid := d.register(&desc)
	if !valid || d.defaultsOnly {
		return Param[T]{value: defaultValue, source: ParamSourceDefault}
	}

	value, source, ok, err := resolveParam(d.inputs, &desc, parser)
	if err != nil {
		d.addError(fmt.Errorf("parameter %q from %s: %w", name, source, err))

		return Param[T]{value: defaultValue, source: ParamSourceDefault}
	}

	if !ok {
		return Param[T]{value: defaultValue, source: ParamSourceDefault}
	}

	return Param[T]{value: value, source: source}
}

func (d *Def) register(desc *paramDescriptor) bool {
	valid := true

	if !paramNamePattern.MatchString(desc.name) {
		d.addError(fmt.Errorf("%w %q: use lower-case kebab-case", errInvalidParamName, desc.name))

		valid = false
	}

	if desc.scope == ParamScopeWorkload {
		if _, reserved := reservedWorkloadParamNames[desc.name]; reserved {
			d.addError(fmt.Errorf("%w %q", errReservedParamName, desc.name))

			valid = false
		}
	}

	if _, duplicate := d.names[desc.name]; duplicate {
		d.addError(fmt.Errorf("%w %q", errDuplicateParamName, desc.name))

		valid = false
	} else {
		d.names[desc.name] = struct{}{}
	}

	seenAliases := make(map[string]struct{}, len(desc.legacyEnvAliases))
	for _, alias := range desc.legacyEnvAliases {
		if !envNamePattern.MatchString(alias) {
			d.addError(fmt.Errorf("parameter %q: %w %q", desc.name, errInvalidLegacyEnvAlias, alias))

			valid = false
		}

		if alias == desc.env {
			d.addError(fmt.Errorf("parameter %q: %w %q", desc.name, errProjectedEnvAlias, alias))

			valid = false
		}

		if _, duplicate := seenAliases[alias]; duplicate {
			d.addError(fmt.Errorf("parameter %q: %w %q", desc.name, errDuplicateLegacyAlias, alias))

			valid = false
		}

		seenAliases[alias] = struct{}{}
	}

	for _, envName := range append([]string{desc.env}, desc.legacyEnvAliases...) {
		if owner, duplicate := d.envNames[envName]; duplicate {
			d.addError(fmt.Errorf(
				"parameter %q: %w: %q belongs to parameter %q",
				desc.name,
				errDuplicateParamEnvName,
				envName,
				owner,
			))

			valid = false
		} else {
			d.envNames[envName] = desc.name
		}
	}

	if valid {
		d.descriptors = append(d.descriptors, *desc)
	}

	return valid
}

func resolveParam[T any](
	inputs ParamInputs,
	desc *paramDescriptor,
	parser paramParser[T],
) (resolved T, source ParamSource, found bool, err error) {
	var zero T

	if value, ok := inputs.CLI[desc.name]; ok {
		parsed, err := parser.text(value)

		return parsed, ParamSourceCLI, true, err
	}

	if value, ok := lookupProcessEnv(desc); ok {
		parsed, err := parser.text(value)

		return parsed, ParamSourceProcessEnv, true, err
	}

	if value, ok := lookupEnvMap(inputs.LegacyEnv, desc); ok {
		parsed, err := parser.text(value)

		return parsed, ParamSourceLegacyEnv, true, err
	}

	config := inputs.WorkloadConfig
	if desc.scope == ParamScopeRun {
		config = inputs.RunConfig
	}

	if value, ok := config[desc.config]; ok {
		parsed, err := parser.raw(value)

		return parsed, ParamSourceConfig, true, err
	}

	if value, ok := lookupEnvMap(inputs.LegacyConfigEnv, desc); ok {
		parsed, err := parser.text(value)

		return parsed, ParamSourceLegacyConfigEnv, true, err
	}

	return zero, ParamSourceDefault, false, nil
}

func lookupProcessEnv(desc *paramDescriptor) (string, bool) {
	if value, ok := os.LookupEnv(desc.env); ok {
		return value, true
	}

	for _, alias := range desc.legacyEnvAliases {
		if value, ok := os.LookupEnv(alias); ok {
			return value, true
		}
	}

	return "", false
}

func lookupEnvMap(values map[string]string, desc *paramDescriptor) (string, bool) {
	if value, ok := values[desc.env]; ok {
		return value, true
	}

	for _, alias := range desc.legacyEnvAliases {
		if value, ok := values[alias]; ok {
			return value, true
		}
	}

	return "", false
}

func decodeJSON[T any](raw json.RawMessage) (T, error) {
	var value T

	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return value, errNullParamValue
	}

	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}

	return value, nil
}

func parseFiniteFloat(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}

	if math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return 0, errNonFiniteParamValue
	}

	return parsed, nil
}

func decodeFiniteFloat(raw json.RawMessage) (float64, error) {
	value, err := decodeJSON[float64](raw)
	if err != nil {
		return 0, err
	}

	if math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, errNonFiniteParamValue
	}

	return value, nil
}

func (d *Def) finish() error {
	for _, name := range sortedKeys(d.inputs.CLI) {
		if _, ok := d.names[name]; !ok {
			d.addError(fmt.Errorf("%w %q", errUnknownCLIParam, name))
		}
	}

	runConfigNames := make(map[string]struct{})
	workloadConfigNames := make(map[string]struct{})

	for idx := range d.descriptors {
		desc := &d.descriptors[idx]
		if desc.scope == ParamScopeRun {
			runConfigNames[desc.config] = struct{}{}
		} else {
			workloadConfigNames[desc.config] = struct{}{}
		}
	}

	for _, name := range sortedKeys(d.inputs.RunConfig) {
		if _, ok := runConfigNames[name]; !ok {
			d.addError(fmt.Errorf("%w %q", errUnknownRunConfigParam, name))
		}
	}

	for _, name := range sortedKeys(d.inputs.WorkloadConfig) {
		if _, ok := workloadConfigNames[name]; !ok {
			d.addError(fmt.Errorf("%w %q", errUnknownWorkloadConfig, name))
		}
	}

	return errors.Join(d.errs...)
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

func (d *Def) schema() []ParamSchema {
	schema := make([]ParamSchema, len(d.descriptors))
	for idx := range d.descriptors {
		desc := &d.descriptors[idx]
		schema[idx] = ParamSchema{
			Name:             desc.name,
			Flag:             "--" + desc.name,
			Scope:            desc.scope,
			Type:             desc.typ,
			Description:      desc.description,
			Default:          desc.defaultValue,
			Env:              desc.env,
			LegacyEnvAliases: slices.Clone(desc.legacyEnvAliases),
			Config:           desc.config,
		}
	}

	slices.SortFunc(schema, func(left, right ParamSchema) int {
		if left.Scope != right.Scope {
			return strings.Compare(string(left.Scope), string(right.Scope))
		}

		return strings.Compare(left.Name, right.Name)
	})

	return schema
}

func (d *Def) addError(err error) {
	if err != nil {
		d.errs = append(d.errs, err)
	}
}

func kebabToEnv(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func kebabToCamel(name string) string {
	parts := strings.Split(name, "-")
	for idx := 1; idx < len(parts); idx++ {
		if parts[idx] != "" {
			parts[idx] = strings.ToUpper(parts[idx][:1]) + parts[idx][1:]
		}
	}

	return strings.Join(parts, "")
}
