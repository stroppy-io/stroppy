package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	stroppydriver "github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type mysqlConnection interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.ExecerContext
	driver.QueryerContext
	driver.Pinger
	driver.NamedValueChecker
	driver.SessionResetter
	driver.Validator
}

var (
	errIsolationInterfaces  = errors.New("mysql connection lacks required context interfaces")
	errIsolationResult      = errors.New("unexpected MySQL session isolation result")
	errIsolationUnsupported = errors.New("unsupported MySQL session isolation")
	isolationChangingSQL    = regexp.MustCompile(`(?is)\bSET\b.*\b(TRANSACTION|transaction_isolation|tx_isolation)\b|` +
		`\b(RESET|EXECUTE)\b`)
)

type isolationConnector struct {
	driver.Connector
	fold             bool
	defaultIsolation config.TxIsolationLevel
}

func (c isolationConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}

	backend, ok := conn.(mysqlConnection)
	if !ok {
		_ = conn.Close()

		return nil, errIsolationInterfaces
	}

	if c.defaultIsolation != config.TxIsolationLevelUnspecified {
		name, levelErr := stroppydriver.IsolationSQL(c.defaultIsolation)
		if levelErr != nil {
			_ = conn.Close()

			return nil, levelErr
		}

		if _, setErr := backend.ExecContext(ctx, "SET SESSION TRANSACTION ISOLATION LEVEL "+name, nil); setErr != nil {
			_ = conn.Close()

			return nil, setErr
		}
	}

	level, err := readSessionIsolation(ctx, backend)
	if errors.Is(err, errIsolationUnsupported) {
		return &isolationConnection{mysqlConnection: backend, fold: c.fold}, nil
	}

	if err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("read MySQL session isolation: %w", err)
	}

	return &isolationConnection{
		mysqlConnection: backend, level: level, known: level != sql.LevelDefault, fold: c.fold,
	}, nil
}

func readSessionIsolation(ctx context.Context, conn driver.QueryerContext) (sql.IsolationLevel, error) {
	rows, err := conn.QueryContext(ctx, "SELECT @@SESSION.transaction_isolation", nil)

	const errUnknownSystemVariable = 1193
	if backend, ok := errors.AsType[*gomysql.MySQLError](err); ok && backend.Number == errUnknownSystemVariable {
		rows, err = conn.QueryContext(ctx, "SELECT @@SESSION.tx_isolation", nil)
	}

	if err != nil {
		return sql.LevelDefault, err
	}

	defer rows.Close()

	values := make([]driver.Value, 1)
	if err := rows.Next(values); err != nil {
		return sql.LevelDefault, err
	}

	level := fmt.Sprint(values[0])
	if text, ok := values[0].([]byte); ok {
		level = string(text)
	}

	if err = rows.Next(values); !errors.Is(err, io.EOF) {
		return sql.LevelDefault, errors.Join(errIsolationResult, err)
	}

	switch strings.ToUpper(strings.ReplaceAll(level, "-", " ")) {
	case "READ UNCOMMITTED":
		return sql.LevelReadUncommitted, nil
	case "READ COMMITTED":
		return sql.LevelReadCommitted, nil
	case "REPEATABLE READ":
		return sql.LevelRepeatableRead, nil
	case "SERIALIZABLE":
		return sql.LevelSerializable, nil
	default:
		return sql.LevelDefault, fmt.Errorf("%w %q", errIsolationUnsupported, level)
	}
}

type isolationConnection struct {
	mysqlConnection
	level   sql.IsolationLevel
	known   bool
	fold    bool
	pending *foldedTransaction
}

func (c *isolationConnection) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.known && sql.IsolationLevel(opts.Isolation) == c.level {
		opts.Isolation = driver.IsolationLevel(sql.LevelDefault)
	}

	if c.fold {
		return c.beginFolded(ctx, opts)
	}

	tx, err := c.mysqlConnection.BeginTx(ctx, opts)
	if err != nil {
		c.known = false
	}

	return tx, err
}

// Hidden routines preserve isolation; visible changes disable the fast path until reconnect.
func (c *isolationConnection) observe(query string) {
	if c.known && isolationChangingSQL.MatchString(query) {
		c.known = false
	}
}

func (c *isolationConnection) ExecContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Result, error) {
	if c.pending != nil && c.pending.startErr != nil {
		return nil, c.pending.startErr
	}

	c.observe(query)
	starting := c.pending != nil && !c.pending.started
	query = c.prefixBegin(query)

	result, err := c.mysqlConnection.ExecContext(ctx, query, args)
	if starting {
		if errors.Is(err, driver.ErrSkip) {
			c.pending.started = false
		} else {
			c.pending.startErr = err
		}
	}

	return result, err
}

func (c *isolationConnection) QueryContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Rows, error) {
	if c.pending != nil && c.pending.startErr != nil {
		return nil, c.pending.startErr
	}

	c.observe(query)
	starting := c.pending != nil && !c.pending.started
	query = c.prefixBegin(query)

	rows, err := c.mysqlConnection.QueryContext(ctx, query, args)
	if starting {
		if errors.Is(err, driver.ErrSkip) {
			c.pending.started = false
		} else {
			c.pending.startErr = err
		}
	}

	return rows, err
}

type mysqlStatement interface {
	driver.Stmt
	driver.StmtExecContext
	driver.StmtQueryContext
	driver.NamedValueChecker
}

type isolationStatement struct {
	mysqlStatement
	connection *isolationConnection
	query      string
}

func (c *isolationConnection) wrapStatement(statement driver.Stmt, query string) (driver.Stmt, error) {
	backend, ok := statement.(mysqlStatement)
	if !ok {
		_ = statement.Close()

		return nil, errIsolationInterfaces
	}

	return &isolationStatement{mysqlStatement: backend, connection: c, query: query}, nil
}

func (s *isolationStatement) Exec(args []driver.Value) (driver.Result, error) {
	if err := s.connection.flushBegin(context.Background()); err != nil {
		return nil, err
	}

	s.connection.observe(s.query)

	return s.mysqlStatement.Exec(args)
}

func (s *isolationStatement) Query(args []driver.Value) (driver.Rows, error) {
	if err := s.connection.flushBegin(context.Background()); err != nil {
		return nil, err
	}

	s.connection.observe(s.query)

	return s.mysqlStatement.Query(args)
}

func (s *isolationStatement) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if s.connection.pending != nil && s.connection.pending.startErr != nil {
		return nil, s.connection.pending.startErr
	}

	if s.connection.pending != nil && !s.connection.pending.started {
		result, err := s.connection.ExecContext(ctx, s.query, args)
		if !errors.Is(err, driver.ErrSkip) {
			return result, err
		}

		if err := s.connection.flushBegin(ctx); err != nil {
			return nil, err
		}
	}

	s.connection.observe(s.query)

	return s.mysqlStatement.ExecContext(ctx, args)
}

func (s *isolationStatement) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if s.connection.pending != nil && s.connection.pending.startErr != nil {
		return nil, s.connection.pending.startErr
	}

	if s.connection.pending != nil && !s.connection.pending.started {
		rows, err := s.connection.QueryContext(ctx, s.query, args)
		if !errors.Is(err, driver.ErrSkip) {
			return rows, err
		}

		if err := s.connection.flushBegin(ctx); err != nil {
			return nil, err
		}
	}

	s.connection.observe(s.query)

	return s.mysqlStatement.QueryContext(ctx, args)
}

func (c *isolationConnection) Prepare(query string) (driver.Stmt, error) {
	statement, err := c.mysqlConnection.Prepare(query)
	if err != nil {
		return nil, err
	}

	return c.wrapStatement(statement, query)
}

func (c *isolationConnection) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	statement, err := c.mysqlConnection.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}

	return c.wrapStatement(statement, query)
}
