package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"time"
)

var errFoldedTransaction = errors.New("mysql transaction already active or closed")

type foldedTransaction struct {
	connection *isolationConnection
	prefix     string
	started    bool
	startErr   error
	done       bool
}

func (c *isolationConnection) beginFolded(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if c.pending != nil {
		return nil, errFoldedTransaction
	}

	prefix := ""

	switch sql.IsolationLevel(opts.Isolation) {
	case sql.LevelDefault:
	case sql.LevelReadUncommitted:
		prefix = "SET TRANSACTION ISOLATION LEVEL READ UNCOMMITTED; "
	case sql.LevelReadCommitted:
		prefix = "SET TRANSACTION ISOLATION LEVEL READ COMMITTED; "
	case sql.LevelRepeatableRead:
		prefix = "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ; "
	case sql.LevelSerializable:
		prefix = "SET TRANSACTION ISOLATION LEVEL SERIALIZABLE; "
	default:
		return c.mysqlConnection.BeginTx(ctx, opts)
	}

	prefix += "START TRANSACTION"
	if opts.ReadOnly {
		prefix += " READ ONLY"
	}

	tx := &foldedTransaction{connection: c, prefix: prefix + "; "}
	c.pending = tx

	return tx, nil
}

func (c *isolationConnection) prefixBegin(query string) string {
	if c.pending == nil || c.pending.started {
		return query
	}

	c.pending.started = true

	return c.pending.prefix + query
}

func (c *isolationConnection) flushBegin(ctx context.Context) error {
	if c.pending != nil && c.pending.startErr != nil {
		return c.pending.startErr
	}

	if c.pending == nil || c.pending.started {
		return nil
	}

	c.pending.started = true
	query := strings.TrimSuffix(c.pending.prefix, "; ")
	_, err := c.mysqlConnection.ExecContext(ctx, query, nil)
	c.pending.startErr = err

	return err
}

func (t *foldedTransaction) Commit() error   { return t.end(true) }
func (t *foldedTransaction) Rollback() error { return t.end(false) }
func (t *foldedTransaction) end(commit bool) error {
	if t.done {
		return errFoldedTransaction
	}

	t.done = true
	c := t.connection
	c.pending = nil

	if !t.started {
		return nil
	}

	command := "ROLLBACK"
	if commit && t.startErr == nil {
		command = "COMMIT"
	}

	const cleanupTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	_, err := c.mysqlConnection.ExecContext(ctx, command, nil)
	if err != nil {
		c.known = false
		_ = c.Close()
	}

	if commit {
		return errors.Join(t.startErr, err)
	}

	return err
}
