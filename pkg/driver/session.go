package driver

import (
	"context"
	"errors"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

var (
	ErrSessionUnsupported = errors.New("driver does not support dedicated prepared sessions")
	ErrSessionClosed      = errors.New("session is closed")
	ErrSessionInvalidated = errors.New("session connection was lost; close the session and prepare again")
	ErrSessionCapacity    = errors.New("dedicated sessions exceed connection pool capacity")
	ErrSessionBusy        = errors.New("session has an open result cursor")
	ErrSessionTransaction = errors.New("session has an active transaction; use the transaction handle")
	ErrSessionMismatch    = errors.New("prepared statement belongs to another session")
	ErrPreparedClosed     = errors.New("prepared statement is closed")
	ErrSessionIsolation   = errors.New("dedicated session transactions require database isolation")
	ErrTransactionClosed  = errors.New("transaction already finalized")
)

// SessionProvider is optional so pooled drivers need not emulate session affinity.
type SessionProvider interface {
	OpenSession(ctx context.Context) (Session, error)
	SessionCapacity() int
}

type Session interface {
	RunQuery(ctx context.Context, sql string, args map[string]any) (*QueryResult, error)
	Prepare(ctx context.Context, name, sql string) (Prepared, error)
	Begin(ctx context.Context, isolation config.TxIsolationLevel) (SessionTx, error)
	Close(ctx context.Context) error
}

type Prepared interface {
	RunQuery(ctx context.Context, args map[string]any) (*QueryResult, error)
	Close(ctx context.Context) error
}

type SessionTx interface {
	Tx
	RunPrepared(ctx context.Context, statement Prepared, args map[string]any) (*QueryResult, error)
}
