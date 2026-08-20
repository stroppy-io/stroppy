// Package execute_sql is the Go-native port of workloads/execute_sql/execute_sql.ts:
// a generic runner that executes every query in a SQL file (or inline SQL string) once.
// The SQL source is one of two typed workload parameters: --sql-file (a path resolved
// cwd → workloads/execute_sql/ → embedded) or --sql-body (inline SQL text). Queries are
// delimited by `--= name` markers, matching parse_sql.ts — a markerless file yields none.
package execute_sql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

var (
	errNoSQLSource      = errors.New("execute_sql: no SQL source — pass --sql-file <path> or --sql-body <inline sql>")
	errSQLFileNoQueries = errors.New("execute_sql: SQL file has no `--= name` queries")
)

type workload struct {
	sql     *bench.SQL
	names   []string
	preset  string
	sqlBody string
	sqlFile string
}

func init() { bench.Register(func() bench.Workload { return &workload{} }) }

func (*workload) Name() string { return "execute_sql" }

func (w *workload) Define(d *bench.Def) error {
	w.sqlBody = d.Param.String(
		"sql-body", "", "Inline SQL to execute.",
		bench.LegacyEnvAliases("STROPPY_SQL_BODY"),
	).Value()
	w.sqlFile = d.Param.String("sql-file", "", "SQL file to execute.").Value()

	return nil
}

func (w *workload) Setup(_ context.Context, b *bench.Bench) error {
	w.preset = "execute_sql"

	switch {
	case w.sqlBody != "":
		w.sql = bench.ParseSQL(w.sqlBody)
	case w.sqlFile != "":
		s, err := bench.LoadSQL(w.preset, w.sqlFile)
		if err != nil {
			return fmt.Errorf("execute_sql: load %s: %w", w.sqlFile, err)
		}

		w.sql = s
	default:
		return errNoSQLSource
	}

	w.names = w.sql.Names("")
	if len(w.names) == 0 {
		return errSQLFileNoQueries
	}

	b.StepBegin("workload")

	return nil
}

func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	return b.Step("workload", func() error {
		lg := b.Logger().Sugar()

		for _, name := range w.names {
			body, ok := w.sql.Query("", name)
			if !ok {
				continue
			}

			start := time.Now()
			err := b.Exec(ctx, body, nil)

			ms := time.Since(start).Milliseconds()
			if err != nil {
				lg.Infof("[execute_sql] %s: error in %dms %v", name, ms, err)

				continue
			}

			lg.Infof("[execute_sql] %s: ok in %dms", name, ms)
		}

		return nil
	})
}

func (*workload) Teardown(_ context.Context, b *bench.Bench) error {
	b.StepEnd("workload")

	return nil
}
