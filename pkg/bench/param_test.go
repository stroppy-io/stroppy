package bench

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestParamResolutionPrecedence(t *testing.T) {
	clearParamEnv(t, "SAMPLE_VALUE", "OLD_SAMPLE_VALUE")

	tests := []struct {
		name       string
		processEnv *string
		inputs     ParamInputs
		want       string
		wantSource ParamSource
	}{
		{
			name: "CLI",
			inputs: ParamInputs{
				CLI:             map[string]string{"sample-value": "cli"},
				LegacyEnv:       map[string]string{"SAMPLE_VALUE": "legacy-env"},
				WorkloadConfig:  map[string]json.RawMessage{"sampleValue": json.RawMessage(`"config"`)},
				LegacyConfigEnv: map[string]string{"SAMPLE_VALUE": "config-env"},
			},
			processEnv: stringPointer("process"),
			want:       "cli",
			wantSource: ParamSourceCLI,
		},
		{
			name:       "process env",
			processEnv: stringPointer("process"),
			inputs: ParamInputs{
				LegacyEnv:       map[string]string{"SAMPLE_VALUE": "legacy-env"},
				WorkloadConfig:  map[string]json.RawMessage{"sampleValue": json.RawMessage(`"config"`)},
				LegacyConfigEnv: map[string]string{"SAMPLE_VALUE": "config-env"},
			},
			want:       "process",
			wantSource: ParamSourceProcessEnv,
		},
		{
			name: "legacy -e",
			inputs: ParamInputs{
				LegacyEnv:       map[string]string{"SAMPLE_VALUE": "legacy-env"},
				WorkloadConfig:  map[string]json.RawMessage{"sampleValue": json.RawMessage(`"config"`)},
				LegacyConfigEnv: map[string]string{"SAMPLE_VALUE": "config-env"},
			},
			want:       "legacy-env",
			wantSource: ParamSourceLegacyEnv,
		},
		{
			name: "typed config",
			inputs: ParamInputs{
				WorkloadConfig:  map[string]json.RawMessage{"sampleValue": json.RawMessage(`"config"`)},
				LegacyConfigEnv: map[string]string{"SAMPLE_VALUE": "config-env"},
			},
			want:       "config",
			wantSource: ParamSourceConfig,
		},
		{
			name: "legacy config env",
			inputs: ParamInputs{
				LegacyConfigEnv: map[string]string{"SAMPLE_VALUE": "config-env"},
			},
			want:       "config-env",
			wantSource: ParamSourceLegacyConfigEnv,
		},
		{name: "default", want: "default", wantSource: ParamSourceDefault},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearParamEnv(t, "SAMPLE_VALUE", "OLD_SAMPLE_VALUE")

			if test.processEnv != nil {
				t.Setenv("SAMPLE_VALUE", *test.processEnv)
			}

			def := newDef(test.inputs, false)
			def.scope = ParamScopeWorkload

			param := def.Param.String(
				"sample-value", "default", "sample", LegacyEnvAliases("OLD_SAMPLE_VALUE"),
			)
			if err := def.finish(); err != nil {
				t.Fatalf("finish() error = %v", err)
			}

			if param.Value() != test.want || param.Source() != test.wantSource {
				t.Fatalf("param = (%q, %q), want (%q, %q)", param.Value(), param.Source(), test.want, test.wantSource)
			}

			if param.Explicit() != (test.wantSource != ParamSourceDefault) {
				t.Fatalf("Explicit() = %v, source = %q", param.Explicit(), param.Source())
			}
		})
	}
}

func TestParamConcreteTypesFromCLI(t *testing.T) {
	clearParamEnv(t, "EMPTY", "ENABLED", "COUNT", "LARGE", "UNSIGNED", "RATIO", "TIMEOUT")

	def := newDef(ParamInputs{CLI: map[string]string{
		"empty":    "",
		"enabled":  "true",
		"count":    "-12",
		"large":    "9223372036854775807",
		"unsigned": "18446744073709551615",
		"ratio":    "1.25",
		"timeout":  "1m30s",
	}}, false)
	def.scope = ParamScopeWorkload

	empty := def.Param.String("empty", "fallback", "")
	enabled := def.Param.Bool("enabled", false, "")
	count := def.Param.Int("count", 0, "")
	large := def.Param.Int64("large", 0, "")
	unsigned := def.Param.Uint64("unsigned", 0, "")
	ratio := def.Param.Float64("ratio", 0, "")
	timeout := def.Param.Duration("timeout", 0, "")

	if err := def.finish(); err != nil {
		t.Fatalf("finish() error = %v", err)
	}

	if empty.Value() != "" || !enabled.Value() || count.Value() != -12 ||
		large.Value() != int64(9223372036854775807) ||
		unsigned.Value() != uint64(18446744073709551615) ||
		ratio.Value() != 1.25 || timeout.Value() != 90*time.Second {
		t.Fatalf("unexpected typed values: %q %v %d %d %d %v %s",
			empty.Value(), enabled.Value(), count.Value(), large.Value(), unsigned.Value(), ratio.Value(), timeout.Value())
	}
}

func TestParamConcreteTypesFromNativeConfig(t *testing.T) {
	clearParamEnv(t, "TEXT", "ENABLED", "COUNT", "LARGE", "UNSIGNED", "RATIO", "TIMEOUT")

	def := newDef(ParamInputs{WorkloadConfig: map[string]json.RawMessage{
		"text":     json.RawMessage(`"value"`),
		"enabled":  json.RawMessage(`true`),
		"count":    json.RawMessage(`-12`),
		"large":    json.RawMessage(`9223372036854775807`),
		"unsigned": json.RawMessage(`18446744073709551615`),
		"ratio":    json.RawMessage(`1.25`),
		"timeout":  json.RawMessage(`"1m30s"`),
	}}, false)
	def.scope = ParamScopeWorkload

	text := def.Param.String("text", "", "")
	enabled := def.Param.Bool("enabled", false, "")
	count := def.Param.Int("count", 0, "")
	large := def.Param.Int64("large", 0, "")
	unsigned := def.Param.Uint64("unsigned", 0, "")
	ratio := def.Param.Float64("ratio", 0, "")
	timeout := def.Param.Duration("timeout", 0, "")

	if err := def.finish(); err != nil {
		t.Fatalf("finish() error = %v", err)
	}

	if text.Value() != "value" || !enabled.Value() || count.Value() != -12 ||
		large.Value() != int64(9223372036854775807) ||
		unsigned.Value() != uint64(18446744073709551615) ||
		ratio.Value() != 1.25 || timeout.Value() != 90*time.Second {
		t.Fatalf("unexpected typed config values")
	}
}

func TestParamRejectsEmptyNonStringAndInvalidConfigTypes(t *testing.T) {
	clearParamEnv(t, "ENABLED", "COUNT", "TIMEOUT", "TEXT")

	tests := []struct {
		name   string
		inputs ParamInputs
		define func(*Def)
		want   string
	}{
		{
			name:   "empty bool",
			inputs: ParamInputs{CLI: map[string]string{"enabled": ""}},
			define: func(def *Def) { def.Param.Bool("enabled", false, "") },
			want:   "invalid syntax",
		},
		{
			name:   "config null",
			inputs: ParamInputs{WorkloadConfig: map[string]json.RawMessage{"count": json.RawMessage(`null`)}},
			define: func(def *Def) { def.Param.Int("count", 1, "") },
			want:   "null is not allowed",
		},
		{
			name:   "config number as string",
			inputs: ParamInputs{WorkloadConfig: map[string]json.RawMessage{"count": json.RawMessage(`"1"`)}},
			define: func(def *Def) { def.Param.Int("count", 1, "") },
			want:   "cannot unmarshal string",
		},
		{
			name:   "config duration as number",
			inputs: ParamInputs{WorkloadConfig: map[string]json.RawMessage{"timeout": json.RawMessage(`10`)}},
			define: func(def *Def) { def.Param.Duration("timeout", time.Second, "") },
			want:   "cannot unmarshal number",
		},
		{
			name:   "config string as bool",
			inputs: ParamInputs{WorkloadConfig: map[string]json.RawMessage{"enabled": json.RawMessage(`"true"`)}},
			define: func(def *Def) { def.Param.Bool("enabled", false, "") },
			want:   "cannot unmarshal string",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			def := newDef(test.inputs, false)
			def.scope = ParamScopeWorkload
			test.define(def)

			err := def.finish()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("finish() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestParamRejectsNonFiniteFloatDefaults(t *testing.T) {
	for name, value := range map[string]float64{
		"nan":      math.NaN(),
		"positive": math.Inf(1),
		"negative": math.Inf(-1),
	} {
		t.Run(name, func(t *testing.T) {
			def := newDef(ParamInputs{}, true)
			def.scope = ParamScopeWorkload
			def.Param.Float64("ratio", value, "")

			err := def.finish()
			if err == nil || !strings.Contains(err.Error(), "must be finite") {
				t.Fatalf("finish() error = %v", err)
			}
		})
	}
}

func TestParamConfigScopesRejectCrossedKeys(t *testing.T) {
	clearScenarioEnv(t)
	clearParamEnv(t, "SCALE_FACTOR")

	workload := &paramTestWorkload{
		name: "test/scoped-config",
		define: func(def *Def) error {
			def.Param.Float64("scale-factor", 1, "")

			return nil
		},
	}

	_, _, err := defineWorkload(workload, ParamInputs{
		RunConfig:      map[string]json.RawMessage{"scaleFactor": json.RawMessage(`2`)},
		WorkloadConfig: map[string]json.RawMessage{"vus": json.RawMessage(`3`)},
	}, false)
	if err == nil {
		t.Fatal("defineWorkload() error = nil")
	}

	if !strings.Contains(err.Error(), `unknown run config parameter "scaleFactor"`) ||
		!strings.Contains(err.Error(), `unknown workload config parameter "vus"`) {
		t.Fatalf("defineWorkload() error = %v", err)
	}
}

func TestDerivedDefaultHidesConcreteSchemaDefault(t *testing.T) {
	def := newDef(ParamInputs{}, true)
	def.scope = ParamScopeWorkload

	binding := def.Param.Int("batch-size", 10, "", DerivedDefault("selected by driver"))
	if err := def.finish(); err != nil {
		t.Fatalf("finish() error = %v", err)
	}

	if binding.Value() != 10 || binding.Source() != ParamSourceDefault {
		t.Fatalf("binding = %#v", binding)
	}

	schema := def.schema()
	if len(schema) != 1 || schema[0].Default != nil || schema[0].DefaultDescription != "selected by driver" {
		t.Fatalf("schema = %#v", schema)
	}
}

func TestParamRejectsIndexedDriverFlagNames(t *testing.T) {
	for _, name := range []string{
		"driver0", "driver1", "driver01", "driver-0", "driver-1",
		"driver0-opt", "driver1-opt", "driver01-opt", "driver-1-opt",
	} {
		t.Run(name, func(t *testing.T) {
			def := newDef(ParamInputs{}, true)
			def.scope = ParamScopeWorkload
			def.Param.String(name, "", "")

			err := def.finish()
			if err == nil || !strings.Contains(err.Error(), "reserved parameter name") {
				t.Fatalf("finish() error = %v", err)
			}
		})
	}

	def := newDef(ParamInputs{}, true)
	def.scope = ParamScopeWorkload
	def.Param.String("driver999999999999999999999999999999", "", "")

	if err := def.finish(); err != nil {
		t.Fatalf("overflowing non-driver index was reserved: %v", err)
	}
}

func TestParamNamesAliasesAndDeferredUnknownChecks(t *testing.T) {
	clearParamEnv(t, "FIRST_VALUE", "SECOND_VALUE", "OLD_SECOND")

	t.Run("later declaration is known", func(t *testing.T) {
		def := newDef(ParamInputs{
			CLI: map[string]string{"second-value": "set"},
		}, false)
		def.scope = ParamScopeWorkload
		first := def.Param.String("first-value", "first", "")
		second := def.Param.String("second-value", "second", "", LegacyEnvAliases("OLD_SECOND"))

		if err := def.finish(); err != nil {
			t.Fatalf("finish() error = %v", err)
		}

		if first.Value() != "first" || second.Value() != "set" {
			t.Fatalf("values = %q, %q", first.Value(), second.Value())
		}
	})

	tests := []struct {
		name   string
		inputs ParamInputs
		define func(*Def)
		want   []string
	}{
		{
			name: "unknown typed inputs sorted",
			inputs: ParamInputs{
				CLI:            map[string]string{"z-last": "1"},
				WorkloadConfig: map[string]json.RawMessage{"aFirst": json.RawMessage(`1`)},
			},
			define: func(*Def) {},
			want:   []string{`unknown CLI parameter "z-last"`, `unknown workload config parameter "aFirst"`},
		},
		{
			name:   "duplicate",
			define: func(def *Def) { def.Param.Int("batch-size", 1, ""); def.Param.Int("batch-size", 2, "") },
			want:   []string{`duplicate parameter name "batch-size"`},
		},
		{
			name:   "reserved",
			define: func(def *Def) { def.Param.String("duration", "", "") },
			want:   []string{`reserved parameter name "duration"`},
		},
		{
			name:   "invalid name",
			define: func(def *Def) { def.Param.String("Bad_Name", "", "") },
			want:   []string{`invalid parameter name "Bad_Name"`},
		},
		{
			name:   "invalid alias",
			define: func(def *Def) { def.Param.String("value", "", "", LegacyEnvAliases("old-value")) },
			want:   []string{`invalid legacy env alias "old-value"`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			def := newDef(test.inputs, false)
			def.scope = ParamScopeWorkload
			test.define(def)

			err := def.finish()
			if err == nil {
				t.Fatal("finish() error = nil")
			}

			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("finish() error = %v, want containing %q", err, want)
				}
			}
		})
	}
}

func TestDescribeUsesDefaultsAndCopiesSchema(t *testing.T) {
	clearParamEnv(t, "BATCH_SIZE", "OLD_BATCH_SIZE", "VUS", "ITERATIONS", "ITER", "DURATION", "EXECUTOR")
	t.Setenv("BATCH_SIZE", "99")
	t.Setenv("VUS", "not-an-int")

	var setupCalls atomic.Int64

	Register(func() Workload {
		return &paramTestWorkload{
			name: "test/describe-params",
			define: func(def *Def) error {
				param := def.Param.Int(
					"batch-size", 10, "Rows in a batch.", LegacyEnvAliases("OLD_BATCH_SIZE"),
				)
				if param.Value() != 10 || param.Source() != ParamSourceDefault {
					return errors.New("description resolved a current value")
				}

				return nil
			},
			setup: func() { setupCalls.Add(1) },
		}
	})

	description, err := Describe("test/describe-params")
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}

	if setupCalls.Load() != 0 {
		t.Fatalf("Setup calls = %d, want 0", setupCalls.Load())
	}

	if description.Name != "test/describe-params" || len(description.Params) != 6 {
		t.Fatalf("description = %#v", description)
	}

	want := ParamSchema{
		Name:             "batch-size",
		Flag:             "--batch-size",
		Scope:            ParamScopeWorkload,
		Type:             ParamTypeInt,
		Description:      "Rows in a batch.",
		Default:          10,
		Env:              "BATCH_SIZE",
		LegacyEnvAliases: []string{"OLD_BATCH_SIZE"},
		Config:           "batchSize",
	}
	if !reflect.DeepEqual(description.Params[5], want) {
		t.Fatalf("workload schema = %#v, want %#v", description.Params[5], want)
	}

	description.Params[5].LegacyEnvAliases[0] = "MUTATED"

	again, err := Describe("test/describe-params")
	if err != nil {
		t.Fatalf("Describe() again error = %v", err)
	}

	if again.Params[5].LegacyEnvAliases[0] != "OLD_BATCH_SIZE" {
		t.Fatalf("schema alias was mutated: %#v", again.Params[5])
	}
}

func TestRegistryFactoriesAreFreshAndDuplicatesPanic(t *testing.T) {
	Register(func() Workload { return &paramTestWorkload{name: "test/fresh-factory"} })

	first, ok := Lookup("test/fresh-factory")
	if !ok {
		t.Fatal("first Lookup() missed registered workload")
	}

	second, ok := Lookup("test/fresh-factory")
	if !ok {
		t.Fatal("second Lookup() missed registered workload")
	}

	if first == second {
		t.Fatal("Lookup() returned the same workload instance")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("duplicate Register() did not panic")
		}
	}()

	Register(func() Workload { return &paramTestWorkload{name: "test/fresh-factory"} })
}

func TestRegisterRejectsTypedNilWorkload(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Register() accepted a typed-nil workload")
		}
	}()

	var workload *paramTestWorkload

	Register(func() Workload { return workload })
}

func TestLookupRejectsTypedNilWorkload(t *testing.T) {
	calls := 0

	Register(func() Workload {
		calls++
		if calls != 2 {
			return &paramTestWorkload{name: "test/typed-nil-lookup"}
		}

		var workload *paramTestWorkload

		return workload
	})

	defer func() {
		if recover() == nil {
			t.Fatal("Lookup() accepted a typed-nil workload")
		}
	}()

	_, _ = Lookup("test/typed-nil-lookup")
}

func TestDescribeAllIsSorted(t *testing.T) {
	descriptions, err := DescribeAll()
	if err != nil {
		t.Fatalf("DescribeAll() error = %v", err)
	}

	for idx := 1; idx < len(descriptions); idx++ {
		if descriptions[idx-1].Name > descriptions[idx].Name {
			t.Fatalf("descriptions are not sorted at %d: %q > %q", idx, descriptions[idx-1].Name, descriptions[idx].Name)
		}
	}
}

func TestScenarioTypedDurationRequiresExplicitConstantVUs(t *testing.T) {
	clearScenarioEnv(t)

	_, err := scenarioForTest(ParamInputs{CLI: map[string]string{"duration": "2s"}}, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "explicit constant-vus") {
		t.Fatalf("typed duration error = %v", err)
	}

	spec, err := scenarioForTest(ParamInputs{CLI: map[string]string{
		"executor": "constant-vus",
		"duration": "2s",
		"vus":      "3",
	}}, zap.NewNop())
	if err != nil {
		t.Fatalf("scenarioForTest() error = %v", err)
	}

	if spec.executor != "constant-vus" || spec.duration != 2*time.Second || spec.vus != 3 {
		t.Fatalf("scenario = %#v", spec)
	}

	spec, err = scenarioForTest(ParamInputs{RunConfig: map[string]json.RawMessage{
		"executor": json.RawMessage(`"constant-vus"`),
		"duration": json.RawMessage(`"3s"`),
	}}, zap.NewNop())
	if err != nil {
		t.Fatalf("config scenario error = %v", err)
	}

	if spec.executor != "constant-vus" || spec.duration != 3*time.Second {
		t.Fatalf("config scenario = %#v", spec)
	}
}

func TestScenarioLegacyDurationInfersConstantVUsWithWarning(t *testing.T) {
	clearScenarioEnv(t)

	core, observed := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	t.Setenv("DURATION", "2s")

	spec, err := scenarioForTest(ParamInputs{}, logger)
	if err != nil {
		t.Fatalf("process env scenario error = %v", err)
	}

	if spec.executor != "constant-vus" || spec.duration != 2*time.Second {
		t.Fatalf("process env scenario = %#v", spec)
	}

	if observed.FilterMessage("legacy DURATION inferred the constant-vus executor; set executor explicitly").Len() != 1 {
		t.Fatalf("warnings = %#v", observed.All())
	}

	clearParamEnv(t, "DURATION")

	for name, inputs := range map[string]ParamInputs{
		"legacy -e":         {LegacyEnv: map[string]string{"DURATION": "3s"}},
		"legacy config env": {LegacyConfigEnv: map[string]string{"DURATION": "4s"}},
	} {
		t.Run(name, func(t *testing.T) {
			legacySpec, err := scenarioForTest(inputs, zap.NewNop())
			if err != nil {
				t.Fatalf("scenario error = %v", err)
			}

			if legacySpec.executor != "constant-vus" {
				t.Fatalf("executor = %q", legacySpec.executor)
			}
		})
	}
}

func TestScenarioLegacyDurationIgnoresLegacyIterations(t *testing.T) {
	clearScenarioEnv(t)

	t.Setenv("DURATION", "2s")
	t.Setenv("ITER", "invalid")

	spec, err := scenarioForTest(ParamInputs{}, zap.NewNop())
	if err != nil {
		t.Fatalf("legacy duration scenario error = %v", err)
	}

	if spec.executor != "constant-vus" || spec.duration != 2*time.Second {
		t.Fatalf("scenario = %#v", spec)
	}

	clearParamEnv(t, "DURATION", "ITER")

	for name, inputs := range map[string]ParamInputs{
		"legacy -e": {
			LegacyEnv: map[string]string{"DURATION": "2s", "ITER": "0"},
		},
		"legacy -e with explicit constant-vus": {
			CLI:       map[string]string{"executor": "constant-vus"},
			LegacyEnv: map[string]string{"DURATION": "2s", "ITER": "invalid"},
		},
		"legacy config env": {
			LegacyConfigEnv: map[string]string{"DURATION": "2s", "ITER": "invalid"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := scenarioForTest(inputs, zap.NewNop()); err != nil {
				t.Fatalf("legacy duration scenario error = %v", err)
			}
		})
	}
}

func TestScenarioLegacyDurationStillValidatesTypedIterations(t *testing.T) {
	clearScenarioEnv(t)

	for name, inputs := range map[string]ParamInputs{
		"CLI": {
			CLI: map[string]string{
				"executor":   "constant-vus",
				"iterations": "0",
			},
			LegacyEnv: map[string]string{"DURATION": "2s"},
		},
		"run config": {
			CLI:       map[string]string{"executor": "constant-vus"},
			RunConfig: map[string]json.RawMessage{"iterations": json.RawMessage(`0`)},
			LegacyEnv: map[string]string{"DURATION": "2s"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := scenarioForTest(inputs, zap.NewNop())
			if err == nil || !strings.Contains(err.Error(), "iterations must be at least 1") {
				t.Fatalf("typed iterations error = %v", err)
			}
		})
	}
}

func TestScenarioValidation(t *testing.T) {
	clearScenarioEnv(t)

	tests := []struct {
		name   string
		cli    map[string]string
		legacy map[string]string
		want   string
	}{
		{name: "vus", cli: map[string]string{"vus": "0"}, want: "vus must be at least 1"},
		{name: "iterations", cli: map[string]string{"iterations": "0"}, want: "iterations must be at least 1"},
		{name: "executor", cli: map[string]string{"executor": "other"}, want: "unsupported executor"},
		{name: "constant needs duration", cli: map[string]string{"executor": "constant-vus"}, want: "requires duration"},
		{
			name: "duration positive",
			cli:  map[string]string{"executor": "constant-vus", "duration": "0s"},
			want: "duration must be positive",
		},
		{
			name:   "explicit shared rejects legacy duration",
			cli:    map[string]string{"executor": "shared-iterations"},
			legacy: map[string]string{"DURATION": "1s"},
			want:   "only valid with the constant-vus",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := scenarioForTest(ParamInputs{CLI: test.cli, LegacyEnv: test.legacy}, zap.NewNop())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("scenario error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRunResolvesParamsBeforeDriverInitialization(t *testing.T) {
	clearScenarioEnv(t)
	clearParamEnv(t, "COUNT")

	Register(func() Workload {
		return &paramTestWorkload{
			name: "test/invalid-before-driver",
			define: func(def *Def) error {
				def.Param.Int("count", 1, "")

				return nil
			},
		}
	})

	err := Run(
		context.Background(),
		"test/invalid-before-driver",
		nil,
		ParamInputs{CLI: map[string]string{"count": "invalid"}},
		nil,
		nil,
		zap.NewNop(),
		&MetricsConfig{},
	)

	if err == nil || !strings.Contains(err.Error(), `parameter "count"`) {
		t.Fatalf("Run() error = %v, want parameter parse error", err)
	}

	if strings.Contains(err.Error(), errDriverIndexMissing.Error()) {
		t.Fatalf("Run() initialized driver before params: %v", err)
	}
}

func scenarioForTest(inputs ParamInputs, logger *zap.Logger) (scenarioSpec, error) {
	params, _, err := defineWorkload(&paramTestWorkload{name: "test/scenario"}, inputs, false)
	if err != nil {
		return scenarioSpec{}, err
	}

	return params.spec(logger)
}

type paramTestWorkload struct {
	name   string
	define func(*Def) error
	setup  func()
}

func (w *paramTestWorkload) Name() string { return w.name }

func (w *paramTestWorkload) Define(def *Def) error {
	if w.define == nil {
		return nil
	}

	return w.define(def)
}

func (w *paramTestWorkload) Setup(context.Context, *Bench) error {
	if w.setup != nil {
		w.setup()
	}

	return nil
}

func (*paramTestWorkload) Iterate(context.Context, *Bench) error  { return nil }
func (*paramTestWorkload) Teardown(context.Context, *Bench) error { return nil }

func clearScenarioEnv(t *testing.T) {
	t.Helper()
	clearParamEnv(t, "EXECUTOR", "VUS", "ITERATIONS", "ITER", "DURATION")
}

func clearParamEnv(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		value, present := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("Unsetenv(%q): %v", name, err)
		}

		t.Cleanup(func() {
			if present {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }
