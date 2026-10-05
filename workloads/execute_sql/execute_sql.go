// Package execute_sql runs named queries from inline or file SQL sources.
package execute_sql

import (
	"context"
	"errors"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

var (
	errNoSQLSource        = errors.New("execute_sql: pass --sql-file or --sql-body")
	errSQLSourceNoQueries = errors.New("execute_sql: SQL source has no named queries")
)

type workload struct {
	sql   *bench.SQL
	names []string
}

var Test = bench.Test{Name: "execute_sql", Define: define}

func init() { bench.Register(Test) }
func define(d *bench.Def) error {
	settings := bench.RunParameters(&d.Param, bench.RunDefaults{})
	body, bodyInfo := d.Param.String("sql-body", "", "Inline SQL to execute.")
	file, fileInfo := d.Param.String("sql-file", "", "SQL file to execute.")
	w := &workload{}

	switch {
	case file != "" && (sqlSourcePriority(fileInfo.Source) > sqlSourcePriority(bodyInfo.Source) || body == ""):
		sql, err := d.Queries.Override(file)
		if err != nil {
			return err
		}

		w.sql = sql
	case body != "":
		w.sql = bench.ParseSQL(body)
	default:
		// Default-only discovery can describe a SQL runner without a source.
		d.Execution.Step("validate_source", w.requireSource)
		d.Execution.Step("workload", w.work, settings.Policy())

		return d.Execution.Err()
	}

	w.names = w.sql.Names("")
	if len(w.names) == 0 {
		return errSQLSourceNoQueries
	}

	d.Execution.Step("workload", w.work, settings.Policy())

	return d.Execution.Err()
}

func sqlSourcePriority(source bench.ParamSource) int {
	switch source {
	case bench.ParamSourceCLI:
		return 3
	case bench.ParamSourceProcessEnv:
		return 2
	case bench.ParamSourceConfig:
		return 1
	default:
		return 0
	}
}

func (w *workload) requireSource(context.Context, *bench.Bench) error {
	if w.sql == nil {
		return errNoSQLSource
	}

	return nil
}

func (w *workload) work(ctx context.Context, b *bench.Bench) error {
	if w.sql == nil {
		return errNoSQLSource
	}

	for _, name := range w.names {
		body, ok := w.sql.Query("", name)
		if !ok {
			continue
		}

		start := time.Now()

		if err := b.Exec(ctx, body, nil); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			b.RecordError(name, err)

			continue
		}

		b.Log.Info("query completed", "query", name, "elapsed", time.Since(start))
	}

	return nil
}
