package mysql

import (
	"context"
	"database/sql"
	sqldriverapi "database/sql/driver"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/sqldriver"
)

var _ driver.SessionProvider = (*Driver)(nil)

func (d *Driver) SessionCapacity() int { return d.db.Stats().MaxOpenConnections }

func (d *Driver) OpenSession(ctx context.Context) (driver.Session, error) {
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, err
	}

	return sqldriver.NewSession(&sessionConnection{conn}, sqldriver.NewRows,
		d.dialect, d.logger, d.queryTimeout), nil
}

type sessionConnection struct{ *sql.Conn }

func (c *sessionConnection) PrepareContext(
	ctx context.Context, query string,
) (sqldriver.PreparedQuery[*sql.Rows], error) {
	statement, err := c.Conn.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}

	return &preparedStatement{statement}, nil
}

func (c *sessionConnection) Begin(
	ctx context.Context, isolation config.TxIsolationLevel,
) (sqldriver.TxConn[*sql.Rows], error) {
	tx, err := c.BeginTx(ctx, &sql.TxOptions{Isolation: sqldriver.IsolationToSQL(isolation)})
	if err != nil {
		return nil, err
	}

	return &sqldriver.SQLTxAdapter{Tx: tx}, nil
}
func (c *sessionConnection) Close(context.Context) error { return c.Conn.Close() }
func (c *sessionConnection) Valid() bool {
	return c.Raw(func(conn any) error {
		if validator, ok := conn.(sqldriverapi.Validator); ok && !validator.IsValid() {
			return sqldriverapi.ErrBadConn
		}

		return nil
	}) == nil
}

type preparedStatement struct{ *sql.Stmt }

func (p *preparedStatement) Close(context.Context) error { return p.Stmt.Close() }
