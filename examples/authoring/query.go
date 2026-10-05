// Package authoring contains small workloads using only the supported author API.
package authoring

import (
	"context"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

const exampleIterations = 10

var Query = bench.Test{Name: "example/query", Define: defineQuery}

func defineQuery(d *bench.Def) error {
	run := bench.RunParameters(&d.Param, bench.RunDefaults{Iterations: exampleIterations})
	d.Execution.Step("query", query, run.Policy())

	return d.Execution.Err()
}

func query(ctx context.Context, b *bench.Bench) error {
	return b.Exec(ctx, "SELECT :value", map[string]any{"value": b.Iteration()})
}
