package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/v6/internal/rowmap"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

var (
	ErrSessionUnsupported = driver.ErrSessionUnsupported
	ErrSessionClosed      = driver.ErrSessionClosed
	ErrSessionInvalidated = driver.ErrSessionInvalidated
	ErrSessionCapacity    = driver.ErrSessionCapacity
	ErrSessionBusy        = driver.ErrSessionBusy
	ErrSessionTransaction = driver.ErrSessionTransaction
	ErrSessionMismatch    = driver.ErrSessionMismatch
	ErrPreparedClosed     = driver.ErrPreparedClosed
)

type Session struct {
	*sessionQueries
	backend     driver.Session
	closed      bool
	transaction *Tx
	rows        map[*Rows]struct{}
}

type Prepared struct {
	*sessionQueries
	backend  driver.Prepared
	owner    *Session
	identity QueryHandle
}

func (p *Prepared) Identity() QueryHandle { return p.identity }

// Session retains one connection until explicit close or step-worker completion.
func (b *Bench) Session(ctx context.Context) (*Session, error) {
	if b.vu.sessions == nil {
		b.vu.sessions = map[string]*Session{}
	}

	name := b.databaseName
	if name == "default" {
		name = ""
	}

	if session := b.vu.sessions[name]; session != nil && !session.closed {
		return session, nil
	}

	if err := b.ensureDriver(ctx); err != nil {
		return nil, err
	}

	provider, ok := b.drv.(driver.SessionProvider)
	if !ok {
		return nil, fmt.Errorf("database %q: %w", name, ErrSessionUnsupported)
	}

	if capacity := provider.SessionCapacity(); capacity > 0 && b.vu.workers > capacity {
		return nil, fmt.Errorf("database %q: %d workers, %d connections: %w",
			name, b.vu.workers, capacity, ErrSessionCapacity)
	}

	acquire, cancel := context.WithTimeout(ctx, defaultCleanupTimeout)
	defer cancel()

	backend, err := provider.OpenSession(b.operationContext(acquire))
	if err != nil {
		return nil, fmt.Errorf("open session %q: %w", name, err)
	}

	s := &Session{backend: backend, rows: map[*Rows]struct{}{}}
	s.sessionQueries = &sessionQueries{b: b, owner: s, run: backend.RunQuery}
	b.vu.sessions[name] = s

	return s, nil
}

func (s *Session) Prepare(ctx context.Context, query QueryHandle) (*Prepared, error) {
	if s.closed {
		return nil, ErrSessionClosed
	}

	backend, err := s.backend.Prepare(s.b.operationContext(ctx), query.Name, query.Text)
	if err != nil {
		return nil, fmt.Errorf("prepare %q: %w", query.Name, err)
	}

	return &Prepared{
		backend: backend, owner: s, identity: query,
		sessionQueries: &sessionQueries{b: s.b, owner: s, run: func(
			ctx context.Context, _ string, args map[string]any,
		) (*driver.QueryResult, error) {
			return backend.RunQuery(ctx, args)
		}, sql: query.Text},
	}, nil
}

func (p *Prepared) Close(ctx context.Context) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultCleanupTimeout)
	defer cancel()

	return p.backend.Close(p.b.operationContext(cleanup))
}

func (s *Session) Begin(ctx context.Context, opts BeginOpts) (*Tx, error) {
	if s.closed {
		return nil, ErrSessionClosed
	}

	iso, err := ParseTxIsolation(string(opts.Isolation))
	if err != nil {
		return nil, err
	}

	backend, err := s.backend.Begin(s.b.operationContext(ctx), iso)
	if err != nil {
		return nil, fmt.Errorf("session begin: %w", err)
	}

	tx := &Tx{tx: backend, b: s.b, iso: iso, name: opts.Name, start: time.Now(), session: s}
	s.transaction = tx

	return tx, nil
}

func (s *Session) BeginTx(ctx context.Context, opts BeginOpts, action func(*Tx) error) (err error) {
	tx, err := s.Begin(ctx, opts)
	if err != nil {
		return err
	}
	defer func() {
		if !tx.done {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultCleanupTimeout)
			defer cancel()

			err = driver.JoinErrors(err, tx.Rollback(cleanup))
		}
	}()

	if err := action(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Session) trackRows(rows *Rows) {
	s.rows[rows] = struct{}{}
	rows.onClose = func() { delete(s.rows, rows) }
}

func (s *Session) closeRows() error {
	var err error
	for rows := range s.rows {
		err = driver.JoinErrors(err, rows.Close())
	}

	return err
}

func (s *Session) Close(ctx context.Context) error {
	if s.closed {
		return nil
	}

	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultCleanupTimeout)
	defer cancel()

	ctx = cleanup

	err := s.closeRows()
	if s.transaction != nil && !s.transaction.done {
		err = driver.JoinErrors(err, s.transaction.Rollback(ctx))
	}

	s.closed = true

	return driver.JoinErrors(err, s.backend.Close(s.b.operationContext(ctx)))
}

func (t *Tx) prepared(statement *Prepared) *sessionQueries {
	return &sessionQueries{
		b: t.b, owner: t.session,
		run: func(ctx context.Context, _ string, args map[string]any) (*driver.QueryResult, error) {
			if t.done {
				return nil, errTransactionClosed
			}

			if statement == nil || t.session == nil || statement.owner != t.session {
				return nil, ErrSessionMismatch
			}

			backend, ok := t.tx.(driver.SessionTx)
			if !ok {
				return nil, ErrSessionUnsupported
			}

			t.queries++

			return backend.RunPrepared(ctx, statement.backend, args)
		},
	}
}

func (t *Tx) ExecPrepared(ctx context.Context, p *Prepared, args map[string]any) error {
	return t.prepared(p).exec(ctx, args)
}

func (t *Tx) QueryPrepared(ctx context.Context, p *Prepared, args map[string]any) (*Rows, error) {
	return t.prepared(p).query(ctx, args)
}

func (t *Tx) QueryRowPrepared(ctx context.Context, p *Prepared, args map[string]any) (Row, error) {
	return t.prepared(p).queryRow(ctx, args)
}

func (t *Tx) QueryValuePrepared[T any](ctx context.Context, p *Prepared, args map[string]any) (T, error) {
	return t.prepared(p).queryValue[T](ctx, args)
}

func (t *Tx) QueryValuesPrepared[T any](ctx context.Context, p *Prepared, args map[string]any) ([]T, error) {
	return t.prepared(p).queryValues[T](ctx, args)
}

func (t *Tx) RawRowPrepared(ctx context.Context, p *Prepared, args map[string]any) ([]any, error) {
	return t.prepared(p).rawRow(ctx, args)
}

func (t *Tx) RawRowsPrepared(ctx context.Context, p *Prepared, args map[string]any) ([][]any, error) {
	return t.prepared(p).rawRows(ctx, args)
}

type sessionQueries struct {
	b     *Bench
	owner *Session
	run   func(context.Context, string, map[string]any) (*driver.QueryResult, error)
	sql   string
}

func (q *sessionQueries) execute(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	if q.owner != nil && q.owner.closed {
		return nil, ErrSessionClosed
	}

	return q.run(q.b.operationContext(ctx), sql, args)
}

func (q *sessionQueries) exec(ctx context.Context, args map[string]any) error {
	result, err := q.execute(ctx, q.sql, args)

	return q.b.finishQuery(result, err)
}

func (q *sessionQueries) query(ctx context.Context, args map[string]any) (*Rows, error) {
	result, err := q.execute(ctx, q.sql, args)
	if err != nil {
		return nil, q.b.finishQuery(result, err)
	}

	rows := newRows(q.b, result)
	if q.owner != nil {
		q.owner.trackRows(rows)
	}

	return rows, nil
}

func (q *sessionQueries) queryRow(ctx context.Context, args map[string]any) (row Row, err error) {
	rows, err := q.query(ctx, args)
	if err != nil {
		return row, err
	}
	defer func() { err = driver.JoinErrors(err, rows.Close()) }()

	if !rows.Next() {
		if rows.Err() != nil {
			return row, rows.Err()
		}

		return row, ErrNoRows
	}

	return rows.Row().Copy(), nil
}

func (q *sessionQueries) queryValue[T any](ctx context.Context, args map[string]any) (T, error) {
	row, err := q.queryRow(ctx, args)
	if err != nil {
		var zero T

		return zero, err
	}

	return rowmap.Decode[T](row.columns, row.values)
}

func (q *sessionQueries) queryValues[T any](ctx context.Context, args map[string]any) (values []T, err error) {
	rows, err := q.query(ctx, args)
	if err != nil {
		return nil, err
	}
	defer func() { err = driver.JoinErrors(err, rows.Close()) }()

	values = make([]T, 0)

	for rows.Next() {
		value, err := rowmap.Decode[T](rows.columns, rows.source.Values())
		if err != nil {
			return nil, err
		}

		values = append(values, value)
	}

	return values, rows.Err()
}

func (q *sessionQueries) rawRow(ctx context.Context, args map[string]any) (_ []any, err error) {
	result, err := q.execute(ctx, q.sql, args)
	defer func() { err = q.b.finishQuery(result, err) }()

	if err != nil {
		return nil, err
	}

	return firstQueryRow(result.Rows)
}

func (q *sessionQueries) rawRows(ctx context.Context, args map[string]any) (_ [][]any, err error) {
	result, err := q.execute(ctx, q.sql, args)
	defer func() { err = q.b.finishQuery(result, err) }()

	if err != nil {
		return nil, err
	}

	return readQueryRows(result.Rows)
}

func (s *Session) queries(sql string) *sessionQueries {
	return &sessionQueries{b: s.b, owner: s, run: s.run, sql: sql}
}

func (s *Session) Exec(ctx context.Context, sql string, args map[string]any) error {
	return s.queries(sql).exec(ctx, args)
}

func (s *Session) Query(ctx context.Context, sql string, args map[string]any) (*Rows, error) {
	return s.queries(sql).query(ctx, args)
}

func (s *Session) QueryRow(ctx context.Context, sql string, args map[string]any) (Row, error) {
	return s.queries(sql).queryRow(ctx, args)
}

func (s *Session) QueryValue[T any](ctx context.Context, sql string, args map[string]any) (T, error) {
	return s.queries(sql).queryValue[T](ctx, args)
}

func (s *Session) QueryValues[T any](ctx context.Context, sql string, args map[string]any) ([]T, error) {
	return s.queries(sql).queryValues[T](ctx, args)
}

func (s *Session) RawRow(ctx context.Context, sql string, args map[string]any) ([]any, error) {
	return s.queries(sql).rawRow(ctx, args)
}

func (s *Session) RawRows(ctx context.Context, sql string, args map[string]any) ([][]any, error) {
	return s.queries(sql).rawRows(ctx, args)
}
func (p *Prepared) Exec(ctx context.Context, args map[string]any) error { return p.exec(ctx, args) }
func (p *Prepared) Query(ctx context.Context, args map[string]any) (*Rows, error) {
	return p.query(ctx, args)
}

func (p *Prepared) QueryRow(ctx context.Context, args map[string]any) (Row, error) {
	return p.queryRow(ctx, args)
}

func (p *Prepared) QueryValue[T any](ctx context.Context, args map[string]any) (T, error) {
	return p.queryValue[T](ctx, args)
}

func (p *Prepared) QueryValues[T any](ctx context.Context, args map[string]any) ([]T, error) {
	return p.queryValues[T](ctx, args)
}

func (p *Prepared) RawRow(ctx context.Context, args map[string]any) ([]any, error) {
	return p.rawRow(ctx, args)
}

func (p *Prepared) RawRows(ctx context.Context, args map[string]any) ([][]any, error) {
	return p.rawRows(ctx, args)
}

func (v *VU) closeSessions(ctx context.Context) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultCleanupTimeout)
	defer cancel()

	var err error

	for name, session := range v.sessions {
		if closeErr := session.Close(cleanup); closeErr != nil {
			err = driver.JoinErrors(err, fmt.Errorf("session %q cleanup: %w", name, closeErr))
		}
	}

	return err
}
