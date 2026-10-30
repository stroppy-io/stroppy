package sqldriver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/sqldriver/queries"
)

type PreparedQuery[R any] interface {
	QueryContext(ctx context.Context, args ...any) (R, error)
	Close(ctx context.Context) error
}

type SessionConnection[R any] interface {
	QueryContext[R]
	PrepareContext(ctx context.Context, sql string) (PreparedQuery[R], error)
	Begin(ctx context.Context, isolation config.TxIsolationLevel) (TxConn[R], error)
	Close(ctx context.Context) error
	Valid() bool
}

type Session[R any] struct {
	mu          sync.Mutex
	conn        SessionConnection[R]
	wrapRows    func(R) driver.Rows
	dialect     queries.Dialect
	logger      *zap.Logger
	timeout     time.Duration
	closed      bool
	invalid     bool
	transaction *sessionTx[R]
	statements  []*prepared[R]
	cursor      *sessionRows[R]
}

func NewSession[R any](conn SessionConnection[R], wrapRows func(R) driver.Rows,
	dialect queries.Dialect, logger *zap.Logger, timeout time.Duration,
) *Session[R] {
	return &Session[R]{conn: conn, wrapRows: wrapRows, dialect: dialect, logger: logger, timeout: timeout}
}

func (s *Session[R]) check(transaction *sessionTx[R]) error {
	switch {
	case s.closed:
		return driver.ErrSessionClosed
	case s.invalid || !s.conn.Valid():
		s.invalid = true

		return driver.ErrSessionInvalidated
	case s.cursor != nil:
		return driver.ErrSessionBusy
	case s.transaction != transaction:
		return driver.ErrSessionTransaction
	default:
		return nil
	}
}

func (s *Session[R]) result(result *driver.QueryResult, err error) (*driver.QueryResult, error) {
	if err != nil {
		if !s.conn.Valid() {
			s.invalid = true
			err = errors.Join(err, driver.ErrSessionInvalidated)
		}

		return result, err
	}

	if result != nil && result.Rows != nil {
		cursor := &sessionRows[R]{Rows: result.Rows, session: s}
		s.cursor = cursor
		result.Rows = cursor
	}

	return result, nil
}

func (s *Session[R]) RunQuery(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.check(nil); err != nil {
		return nil, err
	}

	return s.result(RunQuery(ctx, s.conn, s.wrapRows, s.dialect, s.logger, sql, args, s.timeout))
}

func (s *Session[R]) Prepare(ctx context.Context, _, sql string) (driver.Prepared, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.check(nil); err != nil {
		return nil, err
	}

	processed := lookupOrParse(s.dialect, sql).processedSQL
	processed, _ = s.dialect.StatementTimeoutHint(processed, s.timeout)

	prepareCtx, cancel := StatementTimeout(ctx, s.timeout)
	defer cancel()

	statement, err := s.conn.PrepareContext(prepareCtx, processed)
	if err != nil {
		_, err = s.result(nil, err)

		return nil, fmt.Errorf("prepare: %w", err)
	}

	p := &prepared[R]{session: s, statement: statement, sql: sql}
	s.statements = append(s.statements, p)

	return p, nil
}

func (s *Session[R]) Begin(ctx context.Context, isolation config.TxIsolationLevel) (driver.SessionTx, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.check(nil); err != nil {
		return nil, err
	}

	if isolation == config.TxIsolationLevelNone || isolation == config.TxIsolationLevelConnectionOnly {
		return nil, driver.ErrSessionIsolation
	}

	conn, err := s.conn.Begin(ctx, isolation)
	if err != nil {
		_, err = s.result(nil, err)

		return nil, err
	}

	tx := &sessionTx[R]{session: s, conn: conn, isolation: isolation}
	s.transaction = tx

	return tx, nil
}

func (s *Session[R]) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	var err error
	if s.cursor != nil {
		err = s.cursor.Rows.Close()
		s.cursor.closed = true
		s.cursor = nil
	}

	if s.transaction != nil {
		err = driver.JoinErrors(err, s.transaction.conn.Rollback(ctx))
		s.transaction.done = true
		s.transaction = nil
	}

	for _, statement := range s.statements {
		if !statement.closed {
			statement.closed = true
			err = driver.JoinErrors(err, statement.statement.Close(ctx))
		}
	}

	return driver.JoinErrors(err, s.conn.Close(ctx))
}

type prepared[R any] struct {
	session   *Session[R]
	statement PreparedQuery[R]
	sql       string
	closed    bool
}

func (p *prepared[R]) QueryContext(ctx context.Context, _ string, args ...any) (R, error) {
	return p.statement.QueryContext(ctx, args...)
}

func (p *prepared[R]) run(ctx context.Context, tx *sessionTx[R], args map[string]any) (*driver.QueryResult, error) {
	s := p.session
	s.mu.Lock()
	defer s.mu.Unlock()

	if p.closed {
		return nil, driver.ErrPreparedClosed
	}

	if tx != nil && tx.done {
		return nil, driver.ErrTransactionClosed
	}

	if err := s.check(tx); err != nil {
		return nil, err
	}

	return s.result(RunQuery(ctx, p, s.wrapRows, s.dialect, s.logger, p.sql, args, s.timeout))
}

func (p *prepared[R]) RunQuery(ctx context.Context, args map[string]any) (*driver.QueryResult, error) {
	return p.run(ctx, nil, args)
}

func (p *prepared[R]) Close(ctx context.Context) error {
	s := p.session
	s.mu.Lock()
	defer s.mu.Unlock()

	if p.closed {
		return nil
	}

	if s.cursor != nil {
		return driver.ErrSessionBusy
	}

	p.closed = true

	return p.statement.Close(ctx)
}

type sessionTx[R any] struct {
	session   *Session[R]
	conn      TxConn[R]
	isolation config.TxIsolationLevel
	done      bool
}

func (t *sessionTx[R]) RunQuery(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	s := t.session
	s.mu.Lock()
	defer s.mu.Unlock()

	if t.done {
		return nil, driver.ErrTransactionClosed
	}

	if err := s.check(t); err != nil {
		return nil, err
	}

	return s.result(RunQuery(ctx, t.conn, s.wrapRows, s.dialect, s.logger, sql, args, s.timeout))
}

func (t *sessionTx[R]) RunPrepared(
	ctx context.Context, statement driver.Prepared, args map[string]any,
) (*driver.QueryResult, error) {
	p, ok := statement.(*prepared[R])
	if !ok || p.session != t.session {
		return nil, driver.ErrSessionMismatch
	}

	return p.run(ctx, t, args)
}

func (t *sessionTx[R]) Isolation() config.TxIsolationLevel { return t.isolation }
func (t *sessionTx[R]) Commit(ctx context.Context) error   { return t.end(ctx, true) }
func (t *sessionTx[R]) Rollback(ctx context.Context) error { return t.end(ctx, false) }
func (t *sessionTx[R]) end(ctx context.Context, commit bool) error {
	s := t.session
	s.mu.Lock()
	defer s.mu.Unlock()

	if t.done {
		return driver.ErrTransactionClosed
	}

	if s.cursor != nil {
		return driver.ErrSessionBusy
	}

	t.done = true
	s.transaction = nil

	var err error
	if commit {
		err = t.conn.Commit(ctx)
	} else {
		err = t.conn.Rollback(ctx)
	}

	_, err = s.result(nil, err)

	return err
}

type sessionRows[R any] struct {
	driver.Rows
	session *Session[R]
	closed  bool
	err     error
}

func (r *sessionRows[R]) Next() bool {
	if r.closed {
		return false
	}

	if r.Rows.Next() {
		return true
	}

	r.err = driver.JoinErrors(r.Rows.Err(), r.Close())

	return false
}
func (r *sessionRows[R]) Err() error { return driver.JoinErrors(r.err, r.Rows.Err()) }
func (r *sessionRows[R]) ReadAll(limit int) [][]any {
	result := r.Rows.ReadAll(limit)
	r.err = driver.JoinErrors(r.err, r.Close())

	return result
}

func (r *sessionRows[R]) Close() error {
	s := r.session
	s.mu.Lock()
	defer s.mu.Unlock()

	if !r.closed {
		r.closed = true

		r.err = driver.JoinErrors(r.err, r.Rows.Close())
		if s.cursor == r {
			s.cursor = nil
		}

		if !s.conn.Valid() {
			s.invalid = true
			r.err = driver.JoinErrors(r.err, driver.ErrSessionInvalidated)
		}
	}

	return r.err
}
