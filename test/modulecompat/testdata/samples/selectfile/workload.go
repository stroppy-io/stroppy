// Package workload defines a standalone database stress test.
//
// selectfile loads one named query from a workload-owned SQL file and checks its
// answer against a declared parameter, so the run fails when the database does
// not return what the operator asked for.
package workload

import (
	"context"
	"embed"
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

//go:embed *.go LICENSE README.md queries.sql
var source embed.FS

// queryFile is the workload-owned asset loaded when no override is given.
const queryFile = "queries.sql"

var Test = bench.Test{Name: "selectfile", Define: define, Source: source, SourcePackage: "example.com/selectfile/workload"}

func init() { bench.Register(Test) }

type workload struct {
	handle   bench.QueryHandle
	expected int64
	label    string
}

func define(d *bench.Def) error {
	run := bench.RunParameters(&d.Param, bench.RunDefaults{Iterations: 1})

	w := &workload{}
	w.expected, _ = d.Param.Int64("expected", 1, "Answer the loaded statement must return.", bench.Min(int64(0)))
	w.label, _ = d.Param.String("label", "selectfile", "Value copied into the run report data.")
	sqlFile, _ := d.Param.String("sql-file", "", "SQL file to load instead of the embedded one.")

	queries, err := loadQueries(d, sqlFile)
	if err != nil {
		return err
	}

	w.handle = queries.Require("query", "select_one")

	d.Drivers.Declare("default", bench.DriverConfig{Kind: bench.DriverPostgres})
	d.Execution.Step("query", w.query, run.Policy())

	return d.Execution.Err()
}

func loadQueries(d *bench.Def, sqlFile string) (*bench.SQL, error) {
	if sqlFile != "" {
		return d.Queries.Override(sqlFile)
	}

	return d.Queries.Load(source, queryFile)
}

func (w *workload) query(ctx context.Context, b *bench.Bench) error {
	b.AddReportData("label", w.label)

	value, err := b.QueryValue[int64](ctx, w.handle.Text, nil)
	if err != nil {
		return err
	}

	if value != w.expected {
		return fmt.Errorf("%s returned %d, want %d", w.handle.Name, value, w.expected)
	}

	return nil
}
