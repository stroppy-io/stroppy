package authoring

import (
	"context"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// Mirror sends the same statement to independent primary and secondary databases.
// It makes no distributed-transaction or atomicity guarantee.
var Mirror = bench.Test{Name: "example/mirror", Define: defineMirror}

type mirrorWork struct {
	secondary bench.DriverRef
	writes    *bench.CounterHandle
}

func defineMirror(d *bench.Def) error {
	run := bench.RunParameters(&d.Param, bench.RunDefaults{Iterations: exampleIterations})
	primary := d.Drivers.Declare("primary", bench.DriverConfig{Kind: bench.DriverPostgres})
	secondary := d.Drivers.Declare("secondary", bench.DriverConfig{Kind: bench.DriverPostgres})
	work := mirrorWork{secondary, d.Metrics.Counter("mirror_queries",
		bench.LabelValues("database", "primary", "secondary"))}
	d.Execution.Step("mirror", work.run, run.Policy(), bench.Use(primary))
	d.Report.Put("example.mirror", 1, map[string]string{"atomicity": "independent queries"})

	return d.Execution.Err()
}

func (w *mirrorWork) run(ctx context.Context, b *bench.Bench) error {
	args := map[string]any{"value": b.Iteration()}
	if err := b.Exec(ctx, "SELECT :value", args); err != nil {
		return err
	}

	w.writes.Add(ctx, 1, bench.LabelValue("database", "primary"))

	if err := b.Database(w.secondary).Exec(ctx, "SELECT :value", args); err != nil {
		return err
	}

	w.writes.Add(ctx, 1, bench.LabelValue("database", "secondary"))

	return nil
}
