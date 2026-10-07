package bench

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func clearParamEnv(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		value, present := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}

func clearScenarioEnv(t *testing.T) {
	t.Helper()
	clearParamEnv(
		t,
		"EXECUTOR",
		"VUS",
		"ITERATIONS",
		"DURATION",
		"DRAIN_TIMEOUT",
		"QUERY_TIMEOUT",
	)
}

func TestParameterValueAndProvenance(t *testing.T) {
	clearParamEnv(t, "ROWS", "OLD_ROWS")
	t.Setenv("ROWS", "30")

	d := newDef(ParamInputs{
		CLI:            map[string]string{"old-rows": "40"},
		WorkloadConfig: map[string]json.RawMessage{"rows": json.RawMessage(`20`)},
	}, false)
	value, info := d.Param.Int("rows", 10, "Rows.", Aliases("old-rows"), Min(1))
	require.Equal(t, 40, value)
	require.Equal(t, ParamSourceCLI, info.Source)
	require.Equal(t, "old-rows", info.Spelling)
	require.True(t, info.Explicit())
	require.NoError(t, d.finish())
	d = newDef(ParamInputs{}, false)
	value, info = d.Param.Int("rows", 10, "")
	require.Equal(t, 30, value)
	require.Equal(t, ParamSourceProcessEnv, info.Source)
}

func TestParameterAliasesCanonicalPriority(t *testing.T) {
	clearParamEnv(t, "ROWS", "OLD_ROWS")

	d := newDef(ParamInputs{CLI: map[string]string{"rows": "4", "old-rows": "5"}}, false)
	value, info := d.Param.Int("rows", 1, "", Aliases("old-rows"))
	require.Equal(t, 4, value)
	require.Equal(t, "rows", info.Spelling)
	require.Equal(t, []string{"old-rows"}, d.schema()[0].Aliases)
}

func TestParameterTypesAndVar(t *testing.T) {
	clearParamEnv(t, "COUNT", "ENABLED", "TIMEOUT", "RATIO")

	d := newDef(ParamInputs{CLI: map[string]string{
		"count":   "-12",
		"enabled": "true",
		"timeout": "90s",
		"ratio":   "1.25",
	}}, false)

	var count int64

	info := d.Param.Int64Var(&count, "count", 0, "")
	require.Equal(t, int64(-12), count)
	require.True(t, info.Explicit())

	enabled, _ := d.Param.Bool("enabled", false, "")
	require.True(t, enabled)

	timeout, _ := d.Param.Duration("timeout", 0, "")
	require.Equal(t, 90*time.Second, timeout)

	ratio, _ := d.Param.Declare("ratio", 0., "")
	require.InDelta(t, 1.25, ratio, 0)
	require.NoError(t, d.finish())
}

func TestParameterMalformedInputFailsImmediately(t *testing.T) {
	clearParamEnv(t, "COUNT")

	for _, inputs := range []ParamInputs{
		{CLI: map[string]string{"count": "abc"}},
		{WorkloadConfig: map[string]json.RawMessage{"count": json.RawMessage(`null`)}},
		{WorkloadConfig: map[string]json.RawMessage{"count": json.RawMessage(`"12"`)}},
	} {
		d := newDef(inputs, false)

		require.Panics(t, func() { d.Param.Int("count", 1, "") })
	}
}

func TestParameterInvalidDeclarations(t *testing.T) {
	tests := []func(*Def){
		func(d *Def) { d.Param.Int("Bad_Name", 1, "") },
		func(d *Def) { d.Param.Int("rows", 1, ""); d.Param.Int("rows", 1, "") },
		func(d *Def) { d.Param.Int("rows", 1, "", Aliases("rows")) },
		func(d *Def) { d.Param.Int("rows", 1, "", Min(2)) },
		func(d *Def) { d.Param.Float64("ratio", math.NaN(), "") },
		func(d *Def) { d.Param.IntVar(nil, "rows", 1, "") },
		func(d *Def) { d.Param.String("mode", "other", "", OneOf("known")) },
	}
	for _, test := range tests {
		require.Panics(t, func() { test(newDef(ParamInputs{}, true)) })
	}
}

func TestParameterBoundsKeepIntegerPrecision(t *testing.T) {
	d := newDef(ParamInputs{}, true)
	value, _ := d.Param.Uint64(
		"large",
		uint64(9007199254740993),
		"",
		Min(uint64(9007199254740993)),
	)
	require.Equal(t, uint64(9007199254740993), value)
	require.Panics(t, func() {
		newDef(ParamInputs{}, true).Param.Uint64(
			"large",
			uint64(9007199254740992),
			"",
			Min(uint64(9007199254740993)),
		)
	})
}

func TestParameterUnknownKeys(t *testing.T) {
	d := newDef(ParamInputs{CLI: map[string]string{"typo": "1"}}, false)
	d.Param.Int("rows", 1, "")
	require.ErrorContains(t, d.finish(), "unknown CLI")
	d = newDef(ParamInputs{WorkloadConfig: map[string]json.RawMessage{"typo": json.RawMessage(`1`)}}, false)
	d.Param.Int("rows", 1, "")
	require.ErrorContains(t, d.finish(), "unknown workload")
}

func TestDefaultDescriptionIgnoresEnvironment(t *testing.T) {
	t.Setenv("ROWS", "invalid")

	test := Test{Name: "schema", Define: func(d *Def) error {
		rows, info := d.Param.Int("rows", 10, "Rows.", DerivedDefault("selected by context"))
		require.Equal(t, 10, rows)
		require.False(t, info.Explicit())
		d.Execution.Step("load", func(context.Context, *Bench) error {
			t.Fatal("observation ran action")

			return nil
		})

		return nil
	}}
	description, err := DescribeTest(test)
	require.NoError(t, err)
	require.Nil(t, description.Params[0].Default)
	require.Equal(
		t,
		"selected by context",
		description.Params[0].DefaultDescription,
	)
	require.Len(t, description.Steps, 1)
}

func TestPolicyValidationAndBundle(t *testing.T) {
	clearScenarioEnv(t)
	require.Panics(t, func() { SharedIterations(0, 1) })
	require.Panics(t, func() { ConstantWorkers(1, time.Second, Drain{}) })
	require.Panics(t, func() { DrainTimeout(-1) })
	require.Panics(t, func() { SelectExecutor("unknown", SharedIterations(1, 1)) })

	d := newDef(ParamInputs{CLI: map[string]string{"vus": "3", "iterations": "7"}}, false)
	s := RunParameters(&d.Param, RunDefaults{})
	require.Equal(t, 3, s.Policy().Workers())
	require.Equal(t, int64(7), s.Iterations)
	require.NoError(t, d.finish())
}
