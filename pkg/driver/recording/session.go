package recording

import (
	"context"
	"sync"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

type (
	sessionOperationKey struct{}
	sessionOperation    struct {
		session, statement uint64
		name               string
	}
)

func (*Driver) SessionCapacity() int { return 0 }
func (d *Driver) OpenSession(ctx context.Context) (driver.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s := &session{driver: d, id: d.recorder.Begin(record.ContextScope(ctx))}
	s.event(ctx, "session_open", 0, "", "")

	return s, nil
}

type session struct {
	mu          sync.Mutex
	driver      *Driver
	id          uint64
	closed      bool
	transaction *sessionTransaction
	statements  []*preparedStatement
}

func (s *session) event(ctx context.Context, kind string, statement uint64, name, sql string) {
	s.driver.recorder.Add(record.Operation{
		Scope: record.ContextScope(ctx), Kind: kind,
		Session: s.id, Statement: statement, QueryName: name, SQL: sql,
	})
}

func (s *session) check(ctx context.Context, tx *sessionTransaction) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if s.closed {
		return driver.ErrSessionClosed
	}

	if s.transaction != tx {
		return driver.ErrSessionTransaction
	}

	return nil
}

func (s *session) query(
	ctx context.Context, tx *sessionTransaction, p *preparedStatement, sql string, args map[string]any,
) (*driver.QueryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.check(ctx, tx); err != nil {
		return nil, err
	}

	identity := sessionOperation{session: s.id}

	if p != nil {
		if p.closed {
			return nil, driver.ErrPreparedClosed
		}

		identity.statement, identity.name = p.id, p.name
	}

	var transaction uint64
	if tx != nil {
		transaction = tx.id
	}

	return s.driver.query(context.WithValue(ctx, sessionOperationKey{}, identity), transaction, sql, args)
}

func (s *session) RunQuery(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	return s.query(ctx, nil, nil, sql, args)
}

func (s *session) Prepare(ctx context.Context, name, sql string) (driver.Prepared, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.check(ctx, nil); err != nil {
		return nil, err
	}

	p := &preparedStatement{session: s, id: uint64(len(s.statements) + 1), name: name, sql: sql}
	s.statements = append(s.statements, p)
	s.event(ctx, "prepare", p.id, name, sql)

	return p, nil
}

func (s *session) Begin(ctx context.Context, isolation config.TxIsolationLevel) (driver.SessionTx, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.check(ctx, nil); err != nil {
		return nil, err
	}

	if isolation == config.TxIsolationLevelNone || isolation == config.TxIsolationLevelConnectionOnly {
		return nil, driver.ErrSessionIsolation
	}

	t := &sessionTransaction{session: s, id: s.driver.recorder.Begin(record.ContextScope(ctx)), isolation: isolation}
	s.transaction = t
	s.driver.recorder.Add(record.Operation{
		Scope: record.ContextScope(ctx), Kind: "begin", Session: s.id,
		Transaction: t.id, Isolation: isolationName(isolation),
	})

	return t, nil
}

func (s *session) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	if s.transaction != nil {
		s.transaction.finish(ctx, "rollback")
	}

	for _, p := range s.statements {
		if !p.closed {
			p.closed = true
			s.event(ctx, "statement_close", p.id, p.name, p.sql)
		}
	}

	s.closed = true
	s.event(ctx, "session_close", 0, "", "")

	return nil
}

type preparedStatement struct {
	session   *session
	id        uint64
	name, sql string
	closed    bool
}

func (p *preparedStatement) RunQuery(ctx context.Context, args map[string]any) (*driver.QueryResult, error) {
	return p.session.query(ctx, nil, p, p.sql, args)
}

func (p *preparedStatement) Close(ctx context.Context) error {
	s := p.session
	s.mu.Lock()
	defer s.mu.Unlock()

	if !p.closed {
		p.closed = true
		s.event(ctx, "statement_close", p.id, p.name, p.sql)
	}

	return nil
}

type sessionTransaction struct {
	session   *session
	id        uint64
	isolation config.TxIsolationLevel
	done      bool
}

func (t *sessionTransaction) RunQuery(
	ctx context.Context, sql string, args map[string]any,
) (*driver.QueryResult, error) {
	return t.session.query(ctx, t, nil, sql, args)
}

func (t *sessionTransaction) RunPrepared(
	ctx context.Context, statement driver.Prepared, args map[string]any,
) (*driver.QueryResult, error) {
	p, ok := statement.(*preparedStatement)
	if !ok || p.session != t.session {
		return nil, driver.ErrSessionMismatch
	}

	return t.session.query(ctx, t, p, p.sql, args)
}
func (t *sessionTransaction) Isolation() config.TxIsolationLevel { return t.isolation }
func (t *sessionTransaction) Commit(ctx context.Context) error   { return t.end(ctx, "commit") }
func (t *sessionTransaction) Rollback(ctx context.Context) error { return t.end(ctx, "rollback") }
func (t *sessionTransaction) end(ctx context.Context, kind string) error {
	t.session.mu.Lock()
	defer t.session.mu.Unlock()

	if t.done {
		return driver.ErrTransactionClosed
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	t.finish(ctx, kind)

	return nil
}

func (t *sessionTransaction) finish(ctx context.Context, kind string) {
	t.done = true
	t.session.transaction = nil
	t.session.driver.recorder.Add(record.Operation{
		Scope: record.ContextScope(ctx), Kind: kind, Session: t.session.id, Transaction: t.id,
	})
}
