package mysql

import (
	"context"
	"database/sql"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// stmt wraps a conn-bound *sql.Stmt plus the occurrence-order machinery
// mysql's "?" placeholder requires. database/sql binds one value per "?", and
// Question-mode rendering emits one "?" per :name occurrence, so a named
// [driver.Args] (bound by unique name) is expanded back to occurrence order at
// materialise time.
//
// The *sql.Stmt is bound to the connection that prepared it. mysql drives
// transactions manually (BEGIN/COMMIT as text on the same connection), so a
// conn-bound statement executes inside the open transaction — database/sql's
// *sql.Tx, which forbids conn-prepared statements, is not used.
type stmt struct {
	st      *sql.Stmt
	occur   []string       // per-occurrence param names (with dups)
	uniqIdx map[string]int // name -> position in Params() (AppendTo order)
	args    driver.Args
	scratch []any
}

// Bind returns the statement's reusable argument buffer, reset to empty. The
// buffer is in named mode (the workload contract); fill it with the Set*
// setters, then pass it to a *WithArgs method.
func (s *stmt) Bind() *driver.Args { return s.args.Reset() }

// materialise expands a named [driver.Args] to mysql's per-occurrence "?"
// order. AppendTo yields the unique-order values (Params() order); uniqIdx maps
// each occurrence's name to its unique slot. A nil *Args (no named params)
// leaves the prepared statement argument-less.
func (s *stmt) materialise(a *driver.Args) []any {
	if a == nil || a.Len() == 0 {
		return nil
	}
	uniq := a.AppendTo(s.scratch[:0])
	s.scratch = uniq
	// Occurrence order already equals unique order when no name repeats, or when
	// there are no params — AppendTo's output is the wire order as-is.
	if len(s.occur) == 0 || len(s.occur) == len(uniq) {
		return uniq
	}
	out := make([]any, len(s.occur))
	for i, name := range s.occur {
		out[i] = uniq[s.uniqIdx[name]]
	}
	return out
}

// buildStmt prepares q on cn (the connection every statement for this slot runs
// on) and records the occurrence/unique index the materialiser reads. The Args
// buffer switches to named mode when q carries :params.
func buildStmt(ctx context.Context, cn *sql.Conn, q *sqlfile.Query) (*stmt, error) {
	st, err := cn.PrepareContext(ctx, q.Text(sqlfile.Question))
	if err != nil {
		return nil, err
	}
	s := &stmt{st: st, occur: q.Occurrences(), uniqIdx: driver.BuildNameIndex(q.Params())}
	s.args.SetNames(s.uniqIdx)
	return s, nil
}
