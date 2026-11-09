// Package mpp is what the analytical workloads share about Greenplum-family engines
// (Greenplum, Greengage, WarehousePG, Apache Cloudberry): the dialect name and the check
// that keeps a run on the dialect that matches the server it is connected to.
//
// These engines speak the PostgreSQL protocol, so the pg driver runs them, and the
// pg schema loads and answers queries on them too. It is not the schema they are meant
// to run: without DISTRIBUTED BY every table is spread by its first column and every
// join moves data, so the numbers would be neither representative nor comparable.
// Neither a missing nor a wrong dialect is therefore ever accepted silently.
package mpp

import (
	"context"
	"errors"
	"sync"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// Dialect is the value of the sql-dialect parameter that selects the MPP schema.
const Dialect = "gp"

// probeSQL is true on every Greenplum-family server and false on PostgreSQL: the
// segment catalog is what the family has in common, whatever the product calls itself
// in version() (Greengage says "Greengage Database", not "Greenplum"). to_regclass
// exists since PostgreSQL 9.4, the base of Greenplum 6.
const probeSQL = "SELECT to_regclass('pg_catalog.gp_segment_configuration') IS NOT NULL"

var (
	ErrDriver = errors.New("sql-dialect gp needs the pg driver: Greenplum-family engines speak the PostgreSQL wire")
	ErrFiles  = errors.New("sql-dialect gp brings its own schema; do not combine it with schema-file or sql-file")
	ErrUnlog  = errors.New("pg-unlogged does not apply to sql-dialect gp: Greenplum has no unlogged tables")
	ErrNeedGP = errors.New("the server is a Greenplum-family MPP engine: run with --sql-dialect gp " +
		"(the pg dialect declares no distribution or storage for its tables)")
	ErrNotGP = errors.New("--sql-dialect gp needs a Greenplum-family server: " +
		"this one has no pg_catalog.gp_segment_configuration")
)

// Request is what a workload knows when it picks its dialect.
type Request struct {
	Dialect  string
	Driver   bench.DriverTypeName
	OwnFiles bool // schema-file or sql-file given: the author owns the SQL
	Unlogged bool // pg-unlogged given
}

// Selection is the dialect a run uses and, where it can be checked, the server check.
type Selection struct {
	gp     bool
	verify bool

	once sync.Once
	err  error
}

// Select validates a dialect request at definition time, before any statement runs.
// noop and recording stand in for the pg driver in dry runs and tests.
func Select(r Request) (*Selection, error) {
	s := &Selection{gp: r.Dialect == Dialect}
	if s.gp {
		switch {
		case r.OwnFiles:
			return nil, ErrFiles
		case r.Unlogged:
			return nil, ErrUnlog
		case r.Driver != bench.DriverPostgres && r.Driver != bench.DriverNoop && r.Driver != bench.DriverRecording:
			return nil, ErrDriver
		}
	}

	// Only the pg driver reaches a real server whose family is unknown; other drivers
	// have no probe to run. Own SQL files are the author's responsibility.
	s.verify = r.Driver == bench.DriverPostgres && !r.OwnFiles

	return s, nil
}

// GP reports whether the MPP schema is selected.
func (s *Selection) GP() bool { return s.gp }

// Step wraps a step so the server is checked before the first statement of the run,
// whichever steps the run selects. The check runs once; every step sees its result.
func (s *Selection) Step(action bench.Action) bench.Action {
	if !s.verify {
		return action
	}

	return func(ctx context.Context, b *bench.Bench) error {
		s.once.Do(func() { s.err = s.check(ctx, b) })

		if s.err != nil {
			return s.err
		}

		return action(ctx, b)
	}
}

func (s *Selection) check(ctx context.Context, b *bench.Bench) error {
	isGP, err := b.QueryValue[bool](ctx, probeSQL, nil)
	if err != nil {
		return err
	}

	switch {
	case isGP && !s.gp:
		return ErrNeedGP
	case !isGP && s.gp:
		return ErrNotGP
	default:
		return nil
	}
}
