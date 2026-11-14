package mysql

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionCloseAfterBadConnection(t *testing.T) {
	t.Parallel()

	backend := &isolationTestConnection{}
	db := sql.OpenDB(isolationTestConnector{connection: backend})

	t.Cleanup(func() { require.NoError(t, db.Close()) })

	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	require.ErrorIs(t, conn.Raw(func(any) error { return driver.ErrBadConn }), driver.ErrBadConn)
	require.True(t, backend.closed)

	session := &sessionConnection{conn}
	require.False(t, session.Valid())
	require.NoError(t, session.Close(t.Context()))
}
