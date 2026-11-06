package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/sqldriver"
)

type foldedConnection struct {
	conn      *pgx.Conn
	isolation config.TxIsolationLevel
	started   bool
	startErr  error
	done      bool
	release   func()
}

func (t *foldedConnection) QueryContext(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	if t.done {
		return nil, driver.ErrTransactionClosed
	}

	if t.startErr != nil {
		return nil, t.startErr
	}

	if t.started {
		return t.conn.Query(ctx, query, args...)
	}

	t.started = true
	begin := "BEGIN"

	if t.isolation != config.TxIsolationLevelUnspecified {
		name, err := driver.IsolationSQL(t.isolation)
		if err != nil {
			t.startErr = err

			return nil, err
		}

		begin += " ISOLATION LEVEL " + name
	}

	batch := &pgx.Batch{}
	batch.Queue(begin)
	batch.Queue(query, args...)

	results := t.conn.SendBatch(ctx, batch)
	if _, err := results.Exec(); err != nil {
		t.startErr = driver.JoinErrors(err, results.Close())

		return nil, t.startErr
	}

	rows, err := results.Query()
	if err != nil {
		t.startErr = driver.JoinErrors(err, results.Close())

		return nil, t.startErr
	}

	return &foldedRows{Rows: rows, batch: results}, nil
}

func (t *foldedConnection) QueryPrepared(
	ctx context.Context, statement sqldriver.PreparedQuery[pgx.Rows], args ...any,
) (pgx.Rows, error) {
	prepared, ok := statement.(*preparedStatement)
	if !ok {
		return nil, driver.ErrSessionMismatch
	}

	return t.QueryContext(ctx, prepared.name, args...)
}

func (t *foldedConnection) Commit(ctx context.Context) error   { return t.end(ctx, true) }
func (t *foldedConnection) Rollback(ctx context.Context) error { return t.end(ctx, false) }
func (t *foldedConnection) end(ctx context.Context, commit bool) error {
	if t.done {
		return driver.ErrTransactionClosed
	}

	t.done = true
	defer func() {
		if t.release != nil {
			t.release()
		}
	}()

	if !t.started {
		return nil
	}

	command := "ROLLBACK"
	if commit && t.startErr == nil {
		command = "COMMIT"
	}

	tag, err := t.conn.Exec(ctx, command)
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()

		if _, rollbackErr := t.conn.Exec(cleanup, "ROLLBACK"); rollbackErr != nil {
			_ = t.conn.Close(cleanup)
		}

		return err
	}

	if commit && t.startErr != nil {
		return t.startErr
	}

	if commit && tag.String() == "ROLLBACK" {
		return pgx.ErrTxCommitRollback
	}

	return nil
}

type foldedRows struct {
	pgx.Rows
	batch  pgx.BatchResults
	err    error
	closed bool
}

func (r *foldedRows) Close() {
	if r.closed {
		return
	}

	r.closed = true
	r.Rows.Close()
	r.err = driver.JoinErrors(r.Rows.Err(), r.batch.Close())
}

func (r *foldedRows) Next() bool {
	if r.closed {
		return false
	}

	if r.Rows.Next() {
		return true
	}

	r.Close()

	return false
}
func (r *foldedRows) Err() error { return errors.Join(r.err, r.Rows.Err()) }

func (d *Driver) foldedTx(conn *pgxpool.Conn, isolation config.TxIsolationLevel) driver.Tx {
	return sqldriver.NewTx[pgx.Rows](&foldedConnection{conn: conn.Conn(), isolation: isolation, release: conn.Release},
		NewRows, isolation, PgxDialect{}, d.logger, d.queryTimeout)
}
