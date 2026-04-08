package runner_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/internal/runner"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func TestEffectiveScript(t *testing.T) {
	cfg := &stroppy.RunConfig{Script: proto.String("tpcc")}

	assert.Equal(t, "custom.ts", runner.EffectiveScript("custom.ts", cfg))
	assert.Equal(t, "tpcc", runner.EffectiveScript("", cfg))
	assert.Empty(t, runner.EffectiveScript("", nil))
}

func TestEffectiveSteps(t *testing.T) {
	cfg := &stroppy.RunConfig{Steps: []string{"create_schema", "load"}}

	assert.Equal(t, []string{"only_this"}, runner.EffectiveSteps([]string{"only_this"}, cfg))
	assert.Equal(t, []string{"create_schema", "load"}, runner.EffectiveSteps(nil, cfg))
	assert.Nil(t, runner.EffectiveSteps(nil, nil))
}

func TestEffectiveK6Args(t *testing.T) {
	cfg := &stroppy.RunConfig{K6Args: []string{"--vus", "10"}}

	// file args come first so CLI args override
	result := runner.EffectiveK6Args([]string{"--vus", "20"}, cfg)
	assert.Equal(t, []string{"--vus", "10", "--vus", "20"}, result)

	// only file
	assert.Equal(t, []string{"--vus", "10"}, runner.EffectiveK6Args(nil, cfg))

	// only CLI
	assert.Equal(t, []string{"--dur", "5m"}, runner.EffectiveK6Args([]string{"--dur", "5m"}, nil))
}
