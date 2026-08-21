package runner_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stroppy-io/stroppy/internal/runner"
	"github.com/stroppy-io/stroppy/pkg/config"
)

func TestEffectiveScript(t *testing.T) {
	cfg := &runner.LoadedConfig{RunConfig: &config.RunConfig{Script: ptr("tpcc")}}

	assert.Equal(t, "custom.ts", runner.EffectiveScript("custom.ts", cfg))
	assert.Equal(t, "tpcc", runner.EffectiveScript("", cfg))
	assert.Empty(t, runner.EffectiveScript("", nil))
}

func TestEffectiveSteps(t *testing.T) {
	cfg := &runner.LoadedConfig{RunConfig: &config.RunConfig{Steps: []string{"create_schema", "load"}}}

	assert.Equal(t, []string{"only_this"}, runner.EffectiveSteps([]string{"only_this"}, cfg))
	assert.Equal(t, []string{"create_schema", "load"}, runner.EffectiveSteps(nil, cfg))
	assert.Nil(t, runner.EffectiveSteps(nil, nil))
}
