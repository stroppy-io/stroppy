// Package workload defines a standalone database stress test.
package workload

import (
	"context"
	"embed"
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

//go:embed *.go LICENSE README.md
var source embed.FS

// selectOne is the whole test: one constant statement, no parameters.
const selectOne = "SELECT 1"

var Test = bench.Test{Name: "select1", Define: define, Source: source, SourcePackage: "example.com/select1/workload"}

func init() { bench.Register(Test) }

func define(d *bench.Def) error {
	run := bench.RunParameters(&d.Param, bench.RunDefaults{Iterations: 1})
	d.Drivers.Declare("default", bench.DriverConfig{Kind: bench.DriverPostgres})
	d.Execution.Step("query", query, run.Policy())
	return d.Execution.Err()
}

// query reads the answer instead of discarding it: an iteration passes only when
// the database really returns 1.
func query(ctx context.Context, b *bench.Bench) error {
	value, err := b.QueryValue[int64](ctx, selectOne, nil)
	if err != nil {
		return err
	}

	if value != 1 {
		return fmt.Errorf("%s returned %d, want 1", selectOne, value)
	}

	return nil
}
