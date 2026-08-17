// Package execute_sql is the Go-native port of workloads/execute_sql/execute_sql.ts:
// a generic runner that executes every query in a SQL file (or inline SQL string) once.
// The SQL source is STROPPY_SQL_BODY (inline, CLI-wrapped with a --= marker) or, failing
// that, SQL_FILE (a path resolved cwd → workloads/execute_sql/ → embedded). Queries are
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
	errNoSQLSource      = errors.New("execute_sql: no SQL source — set STROPPY_SQL_BODY (inline) or SQL_FILE (path)")
	errSQLFileNoQueries = errors.New("execute_sql: SQL_FILE has no `--= name` queries")
)

type workload struct {
	sql    *bench.SQL
	names  []string
	preset string
}

func init() { bench.Register(func() bench.Workload { return &workload{} }) }

func (*workload) Name() string { return "execute_sql" }

func (*workload) Define(*bench.Def) error { return nil }

func (w *workload) Setup(_ context.Context, b *bench.Bench) error {
	w.preset = "execute_sql"
	if body := bench.Env("STROPPY_SQL_BODY", ""); body != "" {
		w.sql = bench.ParseSQL(body)
	} else if file := bench.Env("SQL_FILE", ""); file != "" {
		s, err := bench.LoadSQL(w.preset, file)
		if err != nil {
			return fmt.Errorf("execute_sql: load %s: %w", file, err)
		}

		w.sql = s
	} else {
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
