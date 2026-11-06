//go:build integration

package integration

import (
	"context"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type transactionWire struct {
	net.Conn
	writes *atomic.Int64
}

func (c transactionWire) Write(data []byte) (int, error) {
	c.writes.Add(1)

	return c.Conn.Write(data)
}

func TestFoldBeginWireRequests(t *testing.T) {
	for _, backend := range preparedBackends() {
		for _, fold := range []bool{false, true} {
			name := backend.name + "/eager"
			if fold {
				name = backend.name + "/folded"
			}

			t.Run(name, func(t *testing.T) {
				connection := backend.url
				if backend.kind == config.DriverTypePostgres {
					if strings.HasPrefix(connection, "postgres://") || strings.HasPrefix(connection, "postgresql://") {
						parsed, parseErr := url.Parse(connection)
						require.NoError(t, parseErr)

						parameters := parsed.Query()
						parameters.Set("sslmode", "disable")
						parsed.RawQuery = parameters.Encode()
						connection = parsed.String()
					} else {
						connection += " sslmode=disable"
					}
				}

				var writes atomic.Int64

				level := "read_committed"
				db, err := driver.Dispatch(t.Context(), driver.Options{
					Config: &config.DriverConfig{
						DriverType: backend.kind, URL: connection,
						Transactions: &config.TransactionConfig{DefaultIsolation: &level, FoldBegin: &fold},
					}, Logger: zap.NewNop(),
					DialFunc: func(ctx context.Context, network, address string) (net.Conn, error) {
						conn, dialErr := (&net.Dialer{}).DialContext(ctx, network, address)
						if dialErr != nil {
							return nil, dialErr
						}

						return transactionWire{Conn: conn, writes: &writes}, nil
					},
				})
				require.NoError(t, err)

				defer db.Teardown(context.Background())

				session, err := db.(driver.SessionProvider).OpenSession(t.Context())
				require.NoError(t, err)

				defer session.Close(context.Background())

				writes.Store(0)

				tx, err := session.Begin(t.Context(), config.TxIsolationLevelReadCommitted)
				require.NoError(t, err)

				if fold {
					require.Zero(t, writes.Load())
				} else {
					require.Positive(t, writes.Load())
				}

				result, err := tx.RunQuery(t.Context(), "SELECT 1", nil)
				require.EqualValues(t, 1, preparedScalar(t, result, err))

				expected := int64(2)
				if fold {
					expected = 1
				}

				require.Equal(t, expected, writes.Load(), "BEGIN and first query must share one network write")
				require.NoError(t, tx.Rollback(t.Context()))

				prepared, err := session.Prepare(t.Context(), "first", "SELECT :value + 1")
				require.NoError(t, err)

				defer prepared.Close(context.Background())

				writes.Store(0)

				tx, err = session.Begin(t.Context(), config.TxIsolationLevelReadCommitted)
				require.NoError(t, err)
				result, err = tx.RunPrepared(t.Context(), prepared, map[string]any{"value": int64(1)})
				require.EqualValues(t, 2, preparedScalar(t, result, err))
				require.Equal(t, expected, writes.Load(), "prepared first query must share the BEGIN network write")
				require.NoError(t, tx.Rollback(t.Context()))
			})
		}
	}
}
